package analytics_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestJudgeVolume(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		mg   openapi.MuscleGroup
		sets int
		want analytics.VolumeVerdict
	}{
		{"胸 16セットは適正", openapi.Chest, 16, analytics.VolumeOK},
		{"胸 8セットは刺激不足", openapi.Chest, 8, analytics.VolumeBelowMEV},
		{"胸 25セットは回復不足", openapi.Chest, 25, analytics.VolumeAboveMRV},
		// 肩前部はプレス系の巻き込みで積み上がるので上限が低い
		{"肩前部 13セットはMRV超", openapi.FrontDelts, 13, analytics.VolumeAboveMRV},
		{"肩前部 8セットは適正", openapi.FrontDelts, 8, analytics.VolumeOK},
		// 境界
		{"ちょうどMEV", openapi.Chest, 10, analytics.VolumeOK},
		{"ちょうどMRV", openapi.Chest, 20, analytics.VolumeOK},
		{"ゼロ", openapi.Chest, 0, analytics.VolumeBelowMEV},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, r := analytics.JudgeVolume(tt.mg, tt.sets)
			if got != tt.want {
				t.Errorf("JudgeVolume(%q, %d) = %q (MEV-MRV %d-%d), want %q",
					tt.mg, tt.sets, got, r.MEV, r.MRV, tt.want)
			}
		})
	}
}

// 13部位すべてに範囲が定義されていないと、判定が既定値に落ちて意味を失う
func TestVolumeRangeFor_全部位に定義がある(t *testing.T) {
	t.Parallel()

	all := []openapi.MuscleGroup{
		openapi.Chest, openapi.Lats, openapi.Traps,
		openapi.FrontDelts, openapi.SideDelts, openapi.RearDelts,
		openapi.Biceps, openapi.Triceps,
		openapi.Quads, openapi.Hamstrings, openapi.Glutes, openapi.Calves,
		openapi.Abs,
	}
	if len(all) != 13 {
		t.Fatalf("部位が %d 個。13 のはず", len(all))
	}

	for _, mg := range all {
		r := analytics.VolumeRangeFor(mg)
		if r.MEV <= 0 || r.MRV <= r.MEV {
			t.Errorf("%q の範囲が不正: MEV=%d MRV=%d", mg, r.MEV, r.MRV)
		}
	}
}
