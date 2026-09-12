package analytics

// 体組成の分解（要件 A-06）と、月次目標との乖離（要件 A-09）。
// 仕様は docs/03-分析ロジック.md の「分析2」。

// LbmBehindKg は月次目標に対する LBM の遅れとみなす差。
//
// **減量期に LBM が落ちているのは、ペースが速すぎるか回復が足りない。**
// 体重が目標どおりでも、中身が筋肉でなければ計画は失敗している。
const LbmBehindKg = -1.0

// Lbm は除脂肪体重を返す。
func Lbm(weightKg, bodyfatPct float64) float64 {
	return weightKg * (1 - bodyfatPct/100)
}

// CompositionResult は体組成の分解。
type CompositionResult struct {
	LbmKg float64
	Ffmi  float64
}

// Composition は体重・体脂肪率・身長から LBM と正規化FFMI を返す（要件 A-06）。
//
// **体重より先に FFMI を見る。** 体重は身長に強く依存し、他人とも
// 過去の自分とも比較に使えない。
func Composition(weightKg, bodyfatPct, heightCm float64) (CompositionResult, bool) {
	if weightKg <= 0 || heightCm <= 0 || bodyfatPct < 0 || bodyfatPct >= 100 {
		return CompositionResult{}, false
	}

	lbm := Lbm(weightKg, bodyfatPct)

	return CompositionResult{LbmKg: lbm, Ffmi: NormFFMI(lbm, heightCm)}, true
}

// Deviation は月次目標との乖離（要件 A-09）。すべて 実測 − 目標。
type Deviation struct {
	Month      string
	Phase      string
	LbmKg      float64
	BodyfatPct float64
	WeightKg   float64
	Ffmi       float64
	// LbmBehind は LBM が目標を大きく下回っているか
	LbmBehind bool
}

// PlanDeviation は月次目標と実測の差を返す。
//
// 順序は LBM → 体脂肪率 → 体重。**体重の一致は目的ではない**ので、
// 体重が合っていても LBM が落ちていれば遅れとして扱う。
func PlanDeviation(target MonthlyTarget, actual CompositionResult, weightKg, bodyfatPct float64) Deviation {
	lbmDiff := actual.LbmKg - target.LbmKg

	return Deviation{
		Month:      target.Month,
		Phase:      target.Phase,
		LbmKg:      lbmDiff,
		BodyfatPct: bodyfatPct - target.BodyfatPct,
		WeightKg:   weightKg - target.WeightKg,
		Ffmi:       actual.Ffmi - target.Ffmi,
		LbmBehind:  lbmDiff < LbmBehindKg,
	}
}

// FindMonthlyTarget は月（YYYY-MM）に対応する目標を探す。
func FindMonthlyTarget(targets []MonthlyTarget, month string) (MonthlyTarget, bool) {
	for _, t := range targets {
		if t.Month == month {
			return t, true
		}
	}

	return MonthlyTarget{}, false
}
