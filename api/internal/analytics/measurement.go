package analytics

import "math"

// 周囲長から出す指標（要件 A-11）。仕様は docs/03-分析ロジック.md の「分析2」。

// VTaperRatio は V 字が成立するとみなす肩/ウエスト比。
//
// **「映える」の主指標。** 体重や体脂肪率が同じでも、この比が
// 変わると見た目が変わる。
const VTaperRatio = 1.60

// Taper は肩/ウエスト比。
type Taper struct {
	Ratio float64
	// VTaper は V 字が成立するレンジに入っているか
	VTaper bool
}

// ShoulderWaistRatio は肩/ウエスト比を返す。
func ShoulderWaistRatio(shoulderCm, waistCm float64) (Taper, bool) {
	if shoulderCm <= 0 || waistCm <= 0 {
		return Taper{}, false
	}

	r := shoulderCm / waistCm

	return Taper{Ratio: r, VTaper: r >= VTaperRatio}, true
}

// NavyBodyfatSafe は海軍式の体脂肪率推定を返す。計算できない入力では ok に false。
//
// **log10(waist - neck) を含むので、ウエスト ≤ 首 では定義できない。**
// 測り間違いでこの並びになることがあり、NaN や -Inf をそのまま
// 画面に出さないためにここで止める。
func NavyBodyfatSafe(waistCm, neckCm, heightCm float64) (float64, bool) {
	if heightCm <= 0 || waistCm <= neckCm {
		return 0, false
	}

	v := NavyBodyfat(waistCm, neckCm, heightCm)
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return 0, false
	}

	return v, true
}
