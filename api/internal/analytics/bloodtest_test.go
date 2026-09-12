package analytics_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestRefFlag(t *testing.T) {
	t.Parallel()

	lo, hi := 13.0, 17.0

	tests := []struct {
		name  string
		value *float64
		low   *float64
		high  *float64
		want  analytics.RefFlag
	}{
		{"範囲内", ptrF(15), &lo, &hi, analytics.RefNormal},
		{"下限ちょうどは範囲内", ptrF(13), &lo, &hi, analytics.RefNormal},
		{"上限ちょうどは範囲内", ptrF(17), &lo, &hi, analytics.RefNormal},
		{"下回る", ptrF(12.9), &lo, &hi, analytics.RefLow},
		{"上回る", ptrF(17.1), &lo, &hi, analytics.RefHigh},
		// **基準が無ければ判定しない。** 勝手な基準で異常と言わない
		{"基準が無い", ptrF(15), nil, nil, analytics.RefUnknown},
		{"下限だけある（下回る）", ptrF(12), &lo, nil, analytics.RefLow},
		{"下限だけある（上回っても正常）", ptrF(100), &lo, nil, analytics.RefNormal},
		{"上限だけある（上回る）", ptrF(20), nil, &hi, analytics.RefHigh},
		{"値が無い", nil, &lo, &hi, analytics.RefUnknown},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := analytics.JudgeRef(tt.value, tt.low, tt.high); got != tt.want {
				t.Errorf("JudgeRef = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestRefFlag_OutOfRange(t *testing.T) {
	t.Parallel()

	if !analytics.RefLow.OutOfRange() || !analytics.RefHigh.OutOfRange() {
		t.Error("low / high は基準外")
	}
	if analytics.RefNormal.OutOfRange() || analytics.RefUnknown.OutOfRange() {
		t.Error("normal / unknown は基準外ではない")
	}
}
