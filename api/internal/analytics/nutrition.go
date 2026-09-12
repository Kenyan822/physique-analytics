package analytics

// 摂取量とマクロ栄養素の目標（要件 A-02 / A-03）。
// 仕様は docs/03-分析ロジック.md の「分析1」と「分析5（摂取の下限）」。

const (
	// FloorKcalPerKg は摂取量の下限（体重×kcal）。
	//
	// **これを下回る指示を出さない。** 削り続けると LBM 損失と代謝適応が進み、
	// 増量期の立ち上がりが悪くなる。必要な赤字は歩数・有酸素で作る。
	FloorKcalPerKg = 24.0

	kcalPerGProtein = 4.0
	kcalPerGFat     = 9.0
	kcalPerGCarb    = 4.0
)

// NutritionPhase は PFC の適用モード。体脂肪率と増減の方向で決まる。
type NutritionPhase string

const (
	// PhaseCut は減量期。維持（goal = 0）もここに含める。
	PhaseCut NutritionPhase = "cut"
	// PhaseDeepCut は体脂肪率が閾値を下回った減量期。タンパク質を上げる。
	PhaseDeepCut NutritionPhase = "deep_cut"
	// PhaseBulk は増量期。
	PhaseBulk NutritionPhase = "bulk"
)

// Macros は体重あたりのタンパク質・脂質の係数。炭水化物は残余なので持たない。
type Macros struct {
	ProteinGPerKg float64
	FatGPerKg     float64
}

// NutritionConfig は個人設定（private/config.json の nutrition）。
//
// **値をコードに埋めない。** 身長・体重と同じく個人データなので、
// 呼び出し側が設定から渡す（ADR-0002）。
type NutritionConfig struct {
	Cut     Macros
	DeepCut Macros
	Bulk    Macros
	// DeepCutBfThreshold は deep_cut に切り替える体脂肪率（%）
	DeepCutBfThreshold float64
	// CarbMinG は炭水化物の下限。割ったらトレーニングの質が落ちる
	CarbMinG float64
}

// IntakeRecommendation は来週の推奨摂取（要件 A-02 / A-03）。
type IntakeRecommendation struct {
	// TheoreticalKcal は目標ペースから素直に出した値
	TheoreticalKcal float64
	// RecommendedKcal は下限で止めたあとの値。実際に提示するのはこちら
	RecommendedKcal float64
	FloorKcal       float64
	// FloorHit は理論値が下限を下回ったことを表す。
	// 「摂取を削らず消費側で作る」に切り替える合図になる（要件 A-03）
	FloorHit bool
}

// RecommendedIntake は推定TDEE と目標ペースから来週の摂取量を出す。
//
//	理論値 = TDEE + 目標ペース[kg/週] × 7700 / 7
//
// 減量（goal < 0）のときだけ下限を当てる。増量に下限を当てる意味はない。
func RecommendedIntake(tdee, goalKgPerWeek, bodyWeightKg float64) IntakeRecommendation {
	theoretical := tdee + goalKgPerWeek*KcalPerKgFat/daysPerWeek
	floor := bodyWeightKg * FloorKcalPerKg

	out := IntakeRecommendation{
		TheoreticalKcal: theoretical,
		RecommendedKcal: theoretical,
		FloorKcal:       floor,
	}
	if goalKgPerWeek < 0 && theoretical < floor {
		out.RecommendedKcal = floor
		out.FloorHit = true
	}

	return out
}

// MacroTarget は PFC の目標値（要件 A-02）。
type MacroTarget struct {
	Phase    NutritionPhase
	ProteinG float64
	FatG     float64
	// CarbG は残余。摂取枠から P と F を引いた残りで決まる
	CarbG float64
	// CarbBelowFloor は炭水化物の目標が下限を割ったことを表す（要件 A-03）
	CarbBelowFloor bool
}

// MacroTargets は PFC 目標を返す。
//
// タンパク質と脂質は体重から決め、**炭水化物は残余**にする。
// 炭水化物を先に決めると、摂取を絞ったときにタンパク質から削れてしまい、
// 減量期に一番守りたい LBM が落ちる。
//
// bodyfatPct が nil のときは cut として扱う。測れていない値で
// タンパク質を上げる判断をしない。
func MacroTargets(
	cfg NutritionConfig,
	bodyWeightKg float64,
	bodyfatPct *float64,
	goalKgPerWeek float64,
	intakeKcal float64,
) MacroTarget {
	phase := PhaseCut
	macros := cfg.Cut
	switch {
	case goalKgPerWeek > 0:
		phase, macros = PhaseBulk, cfg.Bulk
	case bodyfatPct != nil && *bodyfatPct < cfg.DeepCutBfThreshold:
		phase, macros = PhaseDeepCut, cfg.DeepCut
	}

	protein := bodyWeightKg * macros.ProteinGPerKg
	fat := bodyWeightKg * macros.FatGPerKg
	carb := (intakeKcal - protein*kcalPerGProtein - fat*kcalPerGFat) / kcalPerGCarb
	if carb < 0 {
		// 負の目標は指示にならない。0 まで落ちている時点で下限警告が出る
		carb = 0
	}

	return MacroTarget{
		Phase:          phase,
		ProteinG:       protein,
		FatG:           fat,
		CarbG:          carb,
		CarbBelowFloor: carb < cfg.CarbMinG,
	}
}
