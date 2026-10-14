package timeutil_test

import (
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

func TestStartOfDay(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		// **朝9時前が肝。** Truncate(24h) は絶対時刻を切るのでロケーションを
		// 見ず、JST では9時に丸まる。前日になってしまう（ADR-0013）
		{"朝6時", time.Date(2028, 6, 15, 6, 0, 0, 0, timeutil.JST), "2028-06-15"},
		{"ちょうど0時", time.Date(2028, 6, 15, 0, 0, 0, 0, timeutil.JST), "2028-06-15"},
		{"朝9時ちょうど", time.Date(2028, 6, 15, 9, 0, 0, 0, timeutil.JST), "2028-06-15"},
		{"夜23時59分", time.Date(2028, 6, 15, 23, 59, 59, 0, timeutil.JST), "2028-06-15"},
		// UTC で来ても JST の日付に直す
		{"UTCで来る", time.Date(2028, 6, 14, 21, 0, 0, 0, time.UTC), "2028-06-15"},
		{"UTCで前日", time.Date(2028, 6, 14, 14, 59, 0, 0, time.UTC), "2028-06-14"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got := timeutil.StartOfDay(tt.now)
			if day := got.Format("2006-01-02"); day != tt.want {
				t.Errorf("StartOfDay() = %s, want %s", day, tt.want)
			}
		})
	}
}

func TestStartOfDay_UTCの深夜を返す(t *testing.T) {
	t.Parallel()

	// **date 列は日付そのもの。** +09:00 の深夜を渡すと UTC に直されて
	// 15:00 前日になり、逆方向に1日ずれる
	got := timeutil.StartOfDay(time.Date(2028, 6, 15, 6, 0, 0, 0, timeutil.JST))

	if got.Location() != time.UTC {
		t.Errorf("Location = %v, want UTC", got.Location())
	}
	if h, m, s := got.Clock(); h != 0 || m != 0 || s != 0 {
		t.Errorf("時刻 = %02d:%02d:%02d, want 00:00:00", h, m, s)
	}
}
