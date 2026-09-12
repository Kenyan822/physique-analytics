// Package analytics は分析ロジックを持つ。
//
// **正はこのパッケージ**（ADR-0011）。reference/analysis/analyze.py は
// 移植の検証基準として残してあり、出力が一致することを受け入れ条件にしている。
// 式を変えるときは両方を同時に変える。
//
// 仕様は docs/03-分析ロジック.md。
package analytics

import "math"

const (
	// KcalPerKgFat は体脂肪1kgあたりのカロリー。TDEE の逆算に使う。
	KcalPerKgFat = 7700.0

	// E1RMWindowDays は e1RM の傾きを見る回帰窓（6週）。
	// 短いと日々のばらつきに振り回され、長いと停滞の検出が遅れる。
	E1RMWindowDays = 42

	// E1RMMaxReps は Epley が信頼できる限界レップ数の上限。
	// これを超えると推定1RMを過大評価し、種目内の時系列比較が崩れる。
	E1RMMaxReps = 12

	// epleyDivisor は Epley 式の分母。1RM = w * (1 + r/30)
	epleyDivisor = 30.0

	daysPerWeek = 7.0
)

// E1RM は推定1RMを返す（Epley + RIR補正）。
//
// 限界レップ数 r = reps + rir として w * (1 + r/30)。
// RIR（あと何回できたか）を足すのは、同じ重量×レップでも余力が違えば
// 強度が違うため。RIR が無いと進捗が測れない（openapi.yaml の WorkoutSet.rir）。
//
// UsableForE1RM が false のセットには使わない。
func E1RM(weight, reps, rir float64) float64 {
	return weight * (1 + (reps+rir)/epleyDivisor)
}

// UsableForE1RM は e1RM の計算対象にしてよいセットかを返す。
//
// 除外するのは (a) RIR 未記録 (b) 限界レップ数が E1RMMaxReps 超。
// (b) を残すと Epley が過大評価し、サイドレイズのような高レップ種目が
// 主要種目の伸びを覆い隠す。
func UsableForE1RM(reps int, rir *int) bool {
	if rir == nil {
		return false
	}

	return reps+*rir <= E1RMMaxReps
}

// NormFFMI は正規化FFMI を返す。
//
//	LBM/身長m² + 6.1×(1.8 − 身長m)
//
// 除脂肪体重を身長で正規化し、さらに身長差を補正した筋肉量の指標。
// 18-19=未トレーニング / 20-21=明らかに鍛えている / 22-23=ジムで目立つ /
// 24-25=ナチュラル上限。
//
// **体重より先にこれを見る。** 体重は身長に強く依存し、他人とも過去の自分とも
// 比較に使えない。
func NormFFMI(lbm, heightCm float64) float64 {
	h := heightCm / 100.0

	return lbm/(h*h) + 6.1*(1.8-h)
}

// NavyBodyfat は海軍式の体脂肪率推定（男性）を返す。
//
// 体組成計とは独立した推定値。体組成計は水分量で大きく振れるので、
// 巻尺だけで出せるこの値を併記して傾向を見る。
func NavyBodyfat(waistCm, neckCm, heightCm float64) float64 {
	return 495/(1.0324-0.19077*math.Log10(waistCm-neckCm)+0.15456*math.Log10(heightCm)) - 450
}

// EstimateTDEE は実測から TDEE を逆算する。
//
//	TDEE = 平均摂取 − (体重変化kg/週 × 7700 / 7)
//
// **計算式（Harris-Benedict 等）ではなく実測から求める。** 減量中は代謝適応で
// TDEE が下がっていくが、この方法なら毎週それに追随する。
// slopeKgPerWeek が負（減量中）なら TDEE は摂取より大きくなる。
func EstimateTDEE(avgIntakeKcal, slopeKgPerWeek float64) float64 {
	dailyDeficit := slopeKgPerWeek * KcalPerKgFat / daysPerWeek

	return avgIntakeKcal - dailyDeficit
}

// LinearSlope は最小二乗法で傾きを返す。
//
// 体重トレンド（21日窓）と e1RM の傾き（42日窓）の両方で使う。
// 点が2つ未満、または x がすべて同じ値のときは傾きを決められないので
// ok に false を返す。
func LinearSlope(xs, ys []float64) (slope float64, ok bool) {
	n := float64(len(xs))
	if len(xs) != len(ys) || len(xs) < 2 {
		return 0, false
	}

	var sumX, sumY float64
	for i := range xs {
		sumX += xs[i]
		sumY += ys[i]
	}
	meanX, meanY := sumX/n, sumY/n

	var num, den float64
	for i := range xs {
		dx := xs[i] - meanX
		num += dx * (ys[i] - meanY)
		den += dx * dx
	}
	if den == 0 {
		return 0, false
	}

	return num / den, true
}
