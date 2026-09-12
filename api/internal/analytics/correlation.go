package analytics

import "math"

// 相関分析（要件 A-12）。仕様は docs/03-分析ロジック.md の「分析6」。
//
// **一般論ではなく個人の反応を知るのが目的。** 「睡眠6時間未満だった翌日の
// e1RM は平均でどれくらい落ちるか」は人によって違う。

const (
	// MinCorrelationSamples は相関を信じてよい最低のサンプル数。
	//
	// **これ未満は偶然を拾う。** 30日の相関で行動を変えると、
	// たまたまの並びを法則として扱うことになる。
	MinCorrelationSamples = 90

	// MinPearsonPoints は相関係数を計算できる最低の点数。
	MinPearsonPoints = 3
)

// CorrelationStrength は相関の強さ。絶対値で判定する。
type CorrelationStrength string

const (
	// StrengthNone は相関が無いとみなすレンジ（|r| < 0.2）。
	StrengthNone CorrelationStrength = "none"
	// StrengthWeak は弱い相関（0.2 <= |r| < 0.4）。
	StrengthWeak CorrelationStrength = "weak"
	// StrengthModerate は中程度（0.4 <= |r| < 0.7）。
	StrengthModerate CorrelationStrength = "moderate"
	// StrengthStrong は強い相関（0.7 <= |r|）。
	StrengthStrong CorrelationStrength = "strong"
)

// String は文字列表現を返す。
func (s CorrelationStrength) String() string { return string(s) }

// Strength は相関係数を強さに分類する。
func Strength(r float64) CorrelationStrength {
	a := math.Abs(r)
	switch {
	case a >= 0.7:
		return StrengthStrong
	case a >= 0.4:
		return StrengthModerate
	case a >= 0.2:
		return StrengthWeak
	default:
		return StrengthNone
	}
}

// Correlation は1つの相関の結果。
type Correlation struct {
	// Label は「睡眠 → 翌日の e1RM」のような対応の説明
	Label string
	// N はペアにできたサンプル数
	N int
	// R は相関係数。計算できなければ nil
	R *float64
	// Strength は R の強さ。R が nil なら none
	Strength CorrelationStrength
	// Enough はサンプル数が十分か。false なら参考値として扱う
	Enough bool
}

// Correlate は2つの系列の相関を返す。
//
// **サンプルが足りなくても値は出す。** 隠すと存在に気づかず、
// 「まだ見られない」ことも分からない。足りないことを Enough で示す。
func Correlate(label string, xs, ys []float64) Correlation {
	out := Correlation{
		Label:    label,
		N:        len(xs),
		Strength: StrengthNone,
		Enough:   len(xs) >= MinCorrelationSamples,
	}

	if r, ok := Pearson(xs, ys); ok {
		out.R = &r
		out.Strength = Strength(r)
	}

	return out
}

// Pearson は相関係数を返す。計算できない入力では ok に false。
func Pearson(xs, ys []float64) (float64, bool) {
	if len(xs) != len(ys) || len(xs) < MinPearsonPoints {
		return 0, false
	}

	mx, my := Mean(xs), Mean(ys)

	var sxy, sxx, syy float64
	for i := range xs {
		dx, dy := xs[i]-*mx, ys[i]-*my
		sxy += dx * dy
		sxx += dx * dx
		syy += dy * dy
	}

	// どちらかの分散が0だと相関が定義できない（体重を毎日同じ値で
	// 記録しているようなケース）
	if sxx == 0 || syy == 0 {
		return 0, false
	}

	return sxy / math.Sqrt(sxx*syy), true
}

// LagPairs は「原因の日」と「lag 日後の結果」をペアにする。
//
// 睡眠 → **翌日**の e1RM のように、時間差のある対応を作るために使う。
// 記録の無い日は行ごと欠けるので、日付で突き合わせる。
func LagPairs(causeDays, cause, effectDays, effect []float64, lag float64) (xs, ys []float64) {
	byDay := make(map[float64]float64, len(effectDays))
	for i, d := range effectDays {
		if i < len(effect) {
			byDay[d] = effect[i]
		}
	}

	xs = make([]float64, 0, len(causeDays))
	ys = make([]float64, 0, len(causeDays))
	for i, d := range causeDays {
		if i >= len(cause) {
			break
		}
		if v, ok := byDay[d+lag]; ok {
			xs = append(xs, cause[i])
			ys = append(ys, v)
		}
	}

	return xs, ys
}
