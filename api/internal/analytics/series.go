package analytics

import "math"

// 日次データの集計（要件 A-01 / A-06 / A-07 の前処理）。
//
// **DB から取った生の値を、判定関数が受け取れる形に畳む役。**
// 集計と判定を分けておくと、閾値のテストに時系列データを組み立てずに済む。
//
// 値が足りないときは nil を返す。0 を返すと「測って 0 だった」と
// 区別できず、誤検知になる。

// MinSlopePoints は傾きを出すのに必要な点数。
//
// 体重は日々 ±0.5kg 動くので、少ない点に回帰をかけると
// ノイズをトレンドとして読んでしまう。
const MinSlopePoints = 4

// MinCVPoints は変動係数を出すのに必要な点数（標本標準偏差のため）。
const MinCVPoints = 3

// Mean は平均を返す。空なら nil。
func Mean(vs []float64) *float64 {
	if len(vs) == 0 {
		return nil
	}

	var sum float64
	for _, v := range vs {
		sum += v
	}
	m := sum / float64(len(vs))

	return &m
}

// CVPct は変動係数（標準偏差 / 平均 × 100）を返す。
//
// 摂取kcalが安定しているかの判定に使う。**標本標準偏差（n-1）** を使うのは
// reference の pandas.std() と揃えるため（ADR-0011）。
func CVPct(vs []float64) *float64 {
	if len(vs) < MinCVPoints {
		return nil
	}

	mean := Mean(vs)
	if *mean == 0 {
		return nil
	}

	var ss float64
	for _, v := range vs {
		d := v - *mean
		ss += d * d
	}
	sd := math.Sqrt(ss / float64(len(vs)-1))
	cv := sd / *mean * 100

	return &cv
}

// SlopePerWeek は日単位の系列に回帰をかけ、傾きを週あたりに直して返す。
//
// days は基準日からの経過日数。等間隔でなくてよい（記録の無い日が飛ぶ）。
func SlopePerWeek(days, values []float64) *float64 {
	if len(days) < MinSlopePoints {
		return nil
	}

	perDay, ok := LinearSlope(days, values)
	if !ok {
		return nil
	}
	perWeek := perDay * daysPerWeek

	return &perWeek
}

// MissingDays は期間内の未記録日数を返す。記録が日数を超えても負にしない。
func MissingDays(days, present int) int {
	return max(0, days-present)
}
