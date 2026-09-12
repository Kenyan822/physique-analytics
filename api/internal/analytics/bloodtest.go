package analytics

// 血液検査の基準範囲の判定（要件 B-08）。

// RefFlag は基準範囲に対する位置。
type RefFlag string

const (
	// RefNormal は基準範囲内。
	RefNormal RefFlag = "normal"
	// RefLow は基準を下回る。
	RefLow RefFlag = "low"
	// RefHigh は基準を上回る。
	RefHigh RefFlag = "high"
	// RefUnknown は判定できない（値か基準が無い）。
	RefUnknown RefFlag = "unknown"
)

// OutOfRange は基準外かを返す。
func (f RefFlag) OutOfRange() bool { return f == RefLow || f == RefHigh }

// JudgeRef は値が基準範囲のどこにあるかを返す。
//
// **基準が無ければ判定しない。** 検査項目の基準はクリニックや測定法で
// 違うので、検査票に書いていない基準を勝手に当てて「異常」と言わない。
//
// 片側だけの基準（「40 以下」など）もそのまま扱う。
func JudgeRef(value, low, high *float64) RefFlag {
	if value == nil || (low == nil && high == nil) {
		return RefUnknown
	}

	if low != nil && *value < *low {
		return RefLow
	}
	if high != nil && *value > *high {
		return RefHigh
	}

	return RefNormal
}
