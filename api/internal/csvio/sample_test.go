package csvio_test

import (
	"os"
	"strings"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
)

// data/sample/*.csv をそのまま読めないと、検証基準に食わせられない
func TestサンプルCSVを読める(t *testing.T) {
	cases := []struct {
		file  string
		parse func(*os.File) (int, int, error)
	}{
		{"../../../data/sample/daily.csv", func(f *os.File) (int, int, error) {
			r, e, err := csvio.ParseDaily(f)
			return len(r), len(e), err
		}},
		{"../../../data/sample/workouts.csv", func(f *os.File) (int, int, error) {
			r, e, err := csvio.ParseWorkouts(f)
			return len(r), len(e), err
		}},
		{"../../../data/sample/measures.csv", func(f *os.File) (int, int, error) {
			r, e, err := csvio.ParseMeasures(f)
			return len(r), len(e), err
		}},
	}

	for _, tc := range cases {
		t.Run(strings.TrimPrefix(tc.file, "../../../"), func(t *testing.T) {
			f, err := os.Open(tc.file)
			if err != nil {
				t.Skipf("サンプルが無い: %v", err)
			}
			defer func() { _ = f.Close() }()

			n, nerr, err := tc.parse(f)
			if err != nil {
				t.Fatalf("読めない: %v", err)
			}
			if nerr != 0 {
				t.Errorf("壊れた行が %d 件", nerr)
			}
			if n == 0 {
				t.Error("1行も読めていない")
			}
			t.Logf("%d 行読めた", n)
		})
	}
}
