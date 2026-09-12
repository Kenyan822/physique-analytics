package analytics

import "math"

// 停滞検知（要件 A-07）。仕様は docs/03-分析ロジック.md の「分析5」。
//
// **閾値を先に決めておくことが目的。** 感覚で判断すると、水分変動を
// 停滞と誤認して早すぎる対処をする。

const (
	// PlateauSlopeKgWeek は「体重が横ばい」とみなす傾きの絶対値。
	PlateauSlopeKgWeek = 0.1

	// MeaningfulGoalKgWeek はこの大きさ未満の目標を「維持」とみなす境界。
	// 維持期に横ばいなのは正常なので、停滞判定から外す。
	MeaningfulGoalKgWeek = 0.2

	// StableIntakeCVPct は摂取が安定しているとみなす変動係数（%）。
	// これを超えるばらつきがあるときは、代謝ではなく記録を疑う。
	StableIntakeCVPct = 10.0

	// StrengthDropKgWeek は減量・維持期に筋力低下とみなす e1RM の傾き。
	StrengthDropKgWeek = -0.3

	// BulkStrengthMinKgWeek は増量期に最低限求める e1RM の傾き。
	// 増量中に伸びていないなら、ボリューム・回復・摂取のどれかが足りない。
	BulkStrengthMinKgWeek = 0.0

	// FatigueMax7d は主観的な疲労度（1-5）の7日平均の許容上限。
	FatigueMax7d = 3.5

	// SleepMinHours7d は睡眠時間の7日平均の下限。
	SleepMinHours7d = 6.5

	// HrvDropRatio は HRV の7日平均が30日基準に対して許容される比。
	// 0.85 = -15%。自律神経の疲労は体感より早く出る。
	HrvDropRatio = 0.85

	// RestingHrRiseBpm は安静時心拍の7日平均が基準から上がってよい幅。
	RestingHrRiseBpm = 5.0

	// DeepSleepMinutes7d は深睡眠の7日平均の下限（分）。
	// 睡眠時間が足りていても質が低いことがある。
	DeepSleepMinutes7d = 60.0

	// MissingDaysPerWeek は記録漏れを疑う欠損日数（直近7日）。
	MissingDaysPerWeek = 2

	// StepsDropPerDay は歩数の前週比でこれ以上落ちたら警告する。
	StepsDropPerDay = -1500.0
)

// StallKind は検知の種類。
type StallKind string

const (
	// StallWeightPlateau は減量停滞。摂取が安定したうえで体重が動かない。
	StallWeightPlateau StallKind = "weight_plateau"
	// StallIntakeUnstable は体重が動かないが摂取のばらつきが大きい状態。
	// 停滞ではなく記録精度の問題として扱う。
	StallIntakeUnstable StallKind = "intake_unstable"
	// StallStrengthDrop は減量・維持期の筋力低下。
	StallStrengthDrop StallKind = "strength_drop"
	// StallNoStrengthGain は増量期に e1RM が伸びていない状態。
	// 減量期と閾値を分けるのは、期待される傾きが違うため。
	StallNoStrengthGain StallKind = "no_strength_gain"
	// StallFatigue は主観的な疲労の蓄積。
	StallFatigue StallKind = "fatigue"
	// StallSleepShort は睡眠時間の不足。
	StallSleepShort StallKind = "sleep_short"
	// StallHrvDrop は HRV の低下（客観的な回復不足）。
	StallHrvDrop StallKind = "hrv_drop"
	// StallRestingHrUp は安静時心拍の上昇。
	StallRestingHrUp StallKind = "resting_hr_up"
	// StallDeepSleepShort は深睡眠の不足。
	StallDeepSleepShort StallKind = "deep_sleep_short"
	// StallStepsDrop は歩数の低下。減量停滞の主犯はたいていこれ。
	StallStepsDrop StallKind = "steps_drop"
	// StallMissingRecords は記録漏れ。分析の前提が崩れる。
	StallMissingRecords StallKind = "missing_records"
)

// StallInput は停滞検知の入力。
//
// **集計済みの値だけを受け取る。** 期間の切り方や平均の取り方は呼び出し側の
// 責務にして、この関数は閾値の判定だけを持つ。こうしないと閾値のテストに
// 時系列データを組み立てる必要が出て、仕様と対応が取れなくなる。
//
// nil は「測っていない」。0 と区別する（Apple Watch 未装着の日がある）。
type StallInput struct {
	// GoalKgPerWeek は目標ペース。負なら減量
	GoalKgPerWeek float64
	// WeightSlopeKgWeek は体重トレンドの傾き（21日回帰）
	WeightSlopeKgWeek *float64
	// KcalCVPct は摂取kcalの変動係数（14日, %）
	KcalCVPct *float64
	// E1RMSlopeKgWeek は主要種目の e1RM の傾き（42日回帰）
	E1RMSlopeKgWeek *float64

	Fatigue7dAvg      *float64
	SleepH7dAvg       *float64
	Hrv7dAvg          *float64
	Hrv30dAvg         *float64
	RestingHr7dAvg    *float64
	RestingHr30dAvg   *float64
	DeepSleepMin7dAvg *float64
	Steps7dAvg        *float64
	StepsPrev7dAvg    *float64

	// MissingKcalDays / MissingWeightDays は直近7日の未記録日数
	MissingKcalDays   int
	MissingWeightDays int
}

// Detection は検知1件。判定に使った値を持たせ、文面の組み立て（A-08）に渡す。
type Detection struct {
	Kind StallKind
	// Value は判定に使った実測値。Kind によって単位が変わる
	Value *float64
	// Threshold は超えた閾値。実測値と並べて出すために持つ
	Threshold *float64
}

// DetectStalls は検知した項目を返す。問題が無ければ空。
//
// 返す順序は判定した順で、優先順位は付けない。並べ替えは A-08 が行う。
func DetectStalls(in StallInput) []Detection {
	out := make([]Detection, 0, 4)

	out = appendWeightPlateau(out, in)

	out = appendStrength(out, in)
	if in.Fatigue7dAvg != nil && *in.Fatigue7dAvg > FatigueMax7d {
		out = append(out, detect(StallFatigue, in.Fatigue7dAvg, FatigueMax7d))
	}
	if in.SleepH7dAvg != nil && *in.SleepH7dAvg < SleepMinHours7d {
		out = append(out, detect(StallSleepShort, in.SleepH7dAvg, SleepMinHours7d))
	}

	out = appendRecovery(out, in)

	// 減量中の歩数低下だけを見る。増量中に歩数が落ちても問題にならない
	if in.GoalKgPerWeek < 0 && in.Steps7dAvg != nil && in.StepsPrev7dAvg != nil {
		if drop := *in.Steps7dAvg - *in.StepsPrev7dAvg; drop < StepsDropPerDay {
			out = append(out, detect(StallStepsDrop, &drop, StepsDropPerDay))
		}
	}

	if in.MissingKcalDays >= MissingDaysPerWeek || in.MissingWeightDays >= MissingDaysPerWeek {
		missing := float64(max(in.MissingKcalDays, in.MissingWeightDays))
		out = append(out, detect(StallMissingRecords, &missing, MissingDaysPerWeek))
	}

	return out
}

// appendStrength は e1RM の傾きを見る。
//
// **期待される傾きがフェーズで違う**（docs/03-分析ロジック.md 分析3）。
// 増量期は +0.3〜+1.0kg/週、減量期は -0.1〜0kg/週。同じ閾値で見ると、
// 増量中に横ばいでも見逃し、減量中に少し落ちただけで騒ぐことになる。
func appendStrength(out []Detection, in StallInput) []Detection {
	if in.E1RMSlopeKgWeek == nil {
		return out
	}

	if in.GoalKgPerWeek > 0 {
		if *in.E1RMSlopeKgWeek <= BulkStrengthMinKgWeek {
			return append(out, detect(StallNoStrengthGain, in.E1RMSlopeKgWeek, BulkStrengthMinKgWeek))
		}

		return out
	}

	// 仕様は「-0.3kg/週 以下」なので境界を含める
	if *in.E1RMSlopeKgWeek <= StrengthDropKgWeek {
		return append(out, detect(StallStrengthDrop, in.E1RMSlopeKgWeek, StrengthDropKgWeek))
	}

	return out
}

// appendWeightPlateau は体重が動いていない状態を分類する。
//
// **摂取が安定しているかで結論が変わる。** ばらついているなら、
// 代謝適応ではなく記録漏れを先に疑う（docs/03-分析ロジック.md）。
func appendWeightPlateau(out []Detection, in StallInput) []Detection {
	if in.WeightSlopeKgWeek == nil || math.Abs(in.GoalKgPerWeek) < MeaningfulGoalKgWeek {
		return out
	}
	if math.Abs(*in.WeightSlopeKgWeek) >= PlateauSlopeKgWeek {
		return out
	}

	if in.KcalCVPct != nil && *in.KcalCVPct < StableIntakeCVPct {
		return append(out, detect(StallWeightPlateau, in.WeightSlopeKgWeek, PlateauSlopeKgWeek))
	}

	return append(out, detect(StallIntakeUnstable, in.KcalCVPct, StableIntakeCVPct))
}

// appendRecovery は Apple Watch から入る客観的な回復指標を見る。
//
// 絶対値には大きな個人差があるため、30日基準との乖離で判定する。
func appendRecovery(out []Detection, in StallInput) []Detection {
	if in.Hrv7dAvg != nil && in.Hrv30dAvg != nil && *in.Hrv30dAvg > 0 {
		ratio := *in.Hrv7dAvg / *in.Hrv30dAvg
		if ratio < HrvDropRatio {
			out = append(out, detect(StallHrvDrop, &ratio, HrvDropRatio))
		}
	}
	if in.RestingHr7dAvg != nil && in.RestingHr30dAvg != nil {
		if rise := *in.RestingHr7dAvg - *in.RestingHr30dAvg; rise > RestingHrRiseBpm {
			out = append(out, detect(StallRestingHrUp, &rise, RestingHrRiseBpm))
		}
	}
	if in.DeepSleepMin7dAvg != nil && *in.DeepSleepMin7dAvg < DeepSleepMinutes7d {
		out = append(out, detect(StallDeepSleepShort, in.DeepSleepMin7dAvg, DeepSleepMinutes7d))
	}

	return out
}

func detect(kind StallKind, value *float64, threshold float64) Detection {
	return Detection{Kind: kind, Value: value, Threshold: &threshold}
}
