package analytics_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestRoughMacros_kcal比で按分する(t *testing.T) {
	t.Parallel()

	// P 20% / F 30% / C 50%（kcal 比）。Atwater で g に直す（P4 / F9 / C4）
	tests := []struct {
		name    string
		kcal    int
		p, f, c float64
	}{
		{"1000kcal", 1000, 50, 33.3, 125},
		{"2000kcal", 2000, 100, 66.7, 250},
		{"900kcal（Fが割り切れる）", 900, 45, 30, 112.5},
		{"0kcal", 0, 0, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p, f, c := analytics.RoughMacros(tt.kcal)
			if math.Abs(p-tt.p) > 0.051 || math.Abs(f-tt.f) > 0.051 || math.Abs(c-tt.c) > 0.051 {
				t.Errorf("RoughMacros(%d) = P%v F%v C%v, want P%v F%v C%v", tt.kcal, p, f, c, tt.p, tt.f, tt.c)
			}
		})
	}
}

// **按分した PFC から kcal を再計算しても元の kcal とずれない。**
// 画面は PFC から kcal を出し直す（Atwater）ので、丸めでずれると
// 「1000 と入れたのに 1001 と出る」ことになる。
func TestRoughMacros_再計算してもkcalがずれない(t *testing.T) {
	t.Parallel()

	// DB は numeric(6,1)・API は float32。通した値で確かめる
	for kcal := 0; kcal <= 10000; kcal++ {
		p, f, c := analytics.RoughMacros(kcal)

		if got := analytics.KcalFromMacros(float64(float32(p)), float64(float32(f)), float64(float32(c))); got != kcal {
			t.Fatalf("kcal=%d → P%v F%v C%v → 再計算 %d", kcal, p, f, c, got)
		}
	}
}

func TestRoughMacros_小数1桁で負にならず比率から大きく外れない(t *testing.T) {
	t.Parallel()

	for kcal := 0; kcal <= 10000; kcal++ {
		p, f, c := analytics.RoughMacros(kcal)

		for name, v := range map[string]float64{"P": p, "F": f, "C": c} {
			if v < 0 {
				t.Fatalf("kcal=%d: %s=%v が負", kcal, name, v)
			}
			// 保存先（numeric(6,1)）で丸めても変わらない
			if math.Abs(v*10-math.Round(v*10)) > 1e-9 {
				t.Fatalf("kcal=%d: %s=%v が小数1桁でない", kcal, name, v)
			}
		}
		// 丸めの吸収を F に寄せているので、F だけは比率からわずかに動く（最大 0.1g 程度）
		if math.Abs(p-float64(kcal)*0.05) > 0.051 || math.Abs(c-float64(kcal)*0.125) > 0.051 ||
			math.Abs(f-float64(kcal)*0.3/9) > 0.1 {
			t.Fatalf("kcal=%d: P%v F%v C%v が比率から外れている", kcal, p, f, c)
		}
	}
}
