package analytics_test

import (
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestMealGoalMet(t *testing.T) {
	t.Parallel()

	target := analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}

	tests := []struct {
		name   string
		actual analytics.PFC
		want   bool
	}{
		{"ちょうど目標", analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}, true},
		{"すべて範囲内", analytics.PFC{ProteinG: 185, FatG: 72, CarbG: 240}, true},
		{"下限ちょうど（-10%）は達成", analytics.PFC{ProteinG: 162, FatG: 63, CarbG: 225}, true},
		{"上限ちょうど（+10%）は達成", analytics.PFC{ProteinG: 198, FatG: 77, CarbG: 275}, true},
		{"Fだけ下限割れ", analytics.PFC{ProteinG: 185, FatG: 60, CarbG: 240}, false},
		{"Pだけ上限超え", analytics.PFC{ProteinG: 199, FatG: 70, CarbG: 250}, false},
		{"Cだけ下限割れ", analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 224}, false},
		{"何も食べていない", analytics.PFC{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := analytics.MealGoalMet(target, tt.actual); got != tt.want {
				t.Errorf("MealGoalMet(%+v) = %v, want %v", tt.actual, got, tt.want)
			}
		})
	}
}

func TestMealGoalMet_目標が0なら実績も0のときだけ達成(t *testing.T) {
	t.Parallel()

	zero := analytics.PFC{}
	if !analytics.MealGoalMet(zero, zero) {
		t.Error("0 に対して 0 は達成")
	}
	if analytics.MealGoalMet(zero, analytics.PFC{FatG: 1}) {
		t.Error("0 に対して 1 は未達")
	}
}

func day(t *testing.T, s string) time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatal(err)
	}

	return d
}

func TestBuildStreak(t *testing.T) {
	t.Parallel()

	// 架空の値。10/05 に目標を P150 → P180 に変えた
	targets := []analytics.StreakTarget{
		{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}},
		{StartsOn: "2026-10-05", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}},
	}
	consumed := map[string]analytics.PFC{
		// 旧目標（P150）なら達成、新目標（P180）なら未達
		"2026-10-02": {ProteinG: 150, FatG: 60, CarbG: 200},
		"2026-10-05": {ProteinG: 150, FatG: 60, CarbG: 200},
		"2026-10-06": {ProteinG: 180, FatG: 70, CarbG: 250},
		// 目標が始まる前の記録
		"2026-09-30": {ProteinG: 150, FatG: 60, CarbG: 200},
	}
	trained := map[string]bool{"2026-10-02": true, "2026-10-04": true}

	got := analytics.BuildStreak(day(t, "2026-09-30"), day(t, "2026-10-06"), targets, consumed, trained)

	tr, fa := true, false
	want := []analytics.StreakDay{
		{Date: "2026-09-30", MealGoalMet: nil, Trained: false}, // 目標より前は判定できない（false ではない）
		{Date: "2026-10-01", MealGoalMet: &fa, Trained: false}, // 目標はあるが記録なし
		{Date: "2026-10-02", MealGoalMet: &tr, Trained: true},  // 当時の目標（P150）で達成
		{Date: "2026-10-03", MealGoalMet: &fa, Trained: false},
		{Date: "2026-10-04", MealGoalMet: &fa, Trained: true},
		{Date: "2026-10-05", MealGoalMet: &fa, Trained: false}, // 同じ食事でも新目標（P180）では未達
		{Date: "2026-10-06", MealGoalMet: &tr, Trained: false},
	}
	if len(got) != len(want) {
		t.Fatalf("len = %d, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i].Date != want[i].Date || got[i].Trained != want[i].Trained || !samePtr(got[i].MealGoalMet, want[i].MealGoalMet) {
			t.Errorf("[%d] = %s met=%s trained=%v, want %s met=%s trained=%v", i,
				got[i].Date, show(got[i].MealGoalMet), got[i].Trained,
				want[i].Date, show(want[i].MealGoalMet), want[i].Trained)
		}
	}
}

func TestBuildStreak_履歴が空なら全日null(t *testing.T) {
	t.Parallel()

	got := analytics.BuildStreak(day(t, "2026-10-01"), day(t, "2026-10-03"), nil,
		map[string]analytics.PFC{"2026-10-02": {ProteinG: 180}}, map[string]bool{"2026-10-02": true})

	if len(got) != 3 {
		t.Fatalf("len = %d, want 3", len(got))
	}
	for _, d := range got {
		if d.MealGoalMet != nil {
			t.Errorf("%s: MealGoalMet = %s, want null", d.Date, show(d.MealGoalMet))
		}
	}
	// 目標が無くても筋トレの有無は出る
	if !got[1].Trained {
		t.Error("2026-10-02 は trained")
	}
}

func TestBuildStreak_1日だけ(t *testing.T) {
	t.Parallel()

	d := day(t, "2026-10-01")
	if got := analytics.BuildStreak(d, d, nil, nil, nil); len(got) != 1 {
		t.Errorf("len = %d, want 1", len(got))
	}
}

func samePtr(a, b *bool) bool {
	if a == nil || b == nil {
		return a == b
	}

	return *a == *b
}

func show(p *bool) string {
	if p == nil {
		return "null"
	}
	if *p {
		return "true"
	}

	return "false"
}
