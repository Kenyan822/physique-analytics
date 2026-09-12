package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestShoulderWaistRatio(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name               string
		shoulder, waist    float64
		want               float64
		wantOK, wantVTaper bool
	}{
		// **1.60 以上で V 字が成立する**（reference/analysis/analyze.py）
		{"V字が成立する", 120, 75, 1.6, true, true},
		{"ちょうど1.60も成立", 120, 75, 1.6, true, true},
		{"届いていない", 118, 86, 118.0 / 86.0, true, false},
		{"ウエストが0なら出せない", 120, 0, 0, false, false},
		{"肩が0なら出せない", 0, 75, 0, false, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, ok := analytics.ShoulderWaistRatio(tt.shoulder, tt.waist)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if !ok {
				return
			}
			if math.Abs(got.Ratio-tt.want) > 0.001 {
				t.Errorf("Ratio = %.3f, want %.3f", got.Ratio, tt.want)
			}
			if got.VTaper != tt.wantVTaper {
				t.Errorf("VTaper = %v, want %v", got.VTaper, tt.wantVTaper)
			}
		})
	}
}

func TestNavyBodyfat_出せない入力(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name                string
		waist, neck, height float64
	}{
		// log10(waist - neck) が定義できない
		{"ウエストと首が同じ", 39, 39, 175},
		{"首の方が太い", 38, 39, 175},
		{"身長が0", 86, 39, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if _, ok := analytics.NavyBodyfatSafe(tt.waist, tt.neck, tt.height); ok {
				t.Error("ok = true, want false")
			}
		})
	}
}

func TestNavyBodyfatSafe_通常の入力(t *testing.T) {
	t.Parallel()

	got, ok := analytics.NavyBodyfatSafe(86, 39, 175)
	if !ok {
		t.Fatal("ok = false, want true")
	}
	// 既存の NavyBodyfat と同じ値。式を二重に持たない
	if math.Abs(got-analytics.NavyBodyfat(86, 39, 175)) > 1e-9 {
		t.Errorf("値がずれている: %v", got)
	}
	// 実用域に入っていること（極端な値を返していない）
	if got < 5 || got > 50 {
		t.Errorf("NavyBodyfatSafe = %.1f, 実用域から外れている", got)
	}
}
