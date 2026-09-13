package analytics_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func candidates() []analytics.FoodCandidate {
	return []analytics.FoodCandidate{
		// 高タンパク・低カロリー。残量を埋めるのに向く
		{Name: "サラダチキン", Count: 20, Kcal: 114, ProteinG: 24.1, FatG: 1.5, CarbG: 0.3},
		// 高カロリー。残量が少ないと入らない
		{Name: "カツ丼", Count: 3, Kcal: 900, ProteinG: 30, FatG: 30, CarbG: 120},
		// タンパク質が無い
		{Name: "白米", Count: 30, Kcal: 336, ProteinG: 5, FatG: 0.6, CarbG: 74},
		{Name: "プロテイン", Count: 40, Kcal: 120, ProteinG: 24, FatG: 1.5, CarbG: 2},
	}
}

func TestSuggestFoods_タンパク質の不足を埋めるものを上に出す(t *testing.T) {
	t.Parallel()

	// あと 600kcal / P 50g
	rem := analytics.Remaining{Kcal: 600, ProteinG: 50, FatG: 20, CarbG: 60}

	got := analytics.SuggestFoods(rem, candidates(), 3)

	if len(got) == 0 {
		t.Fatal("提案が空")
	}
	// **タンパク質を優先する。** 減量期に一番守りたいのが LBM で、
	// 足りないと分かっていても最後に回されがち
	if got[0].Name != "サラダチキン" && got[0].Name != "プロテイン" {
		t.Errorf("先頭 = %q, want サラダチキン か プロテイン", got[0].Name)
	}
	// カロリーを大きく超えるものは上に来ない
	for i, s := range got {
		if s.Name == "カツ丼" && i == 0 {
			t.Error("カロリーを超えるものが先頭に来ている")
		}
	}
}

func TestSuggestFoods_残量に入るかを示す(t *testing.T) {
	t.Parallel()

	// あと 200kcal しかない
	rem := analytics.Remaining{Kcal: 200, ProteinG: 40, FatG: 10, CarbG: 10}

	got := analytics.SuggestFoods(rem, candidates(), 10)

	byName := map[string]analytics.FoodSuggestion{}
	for _, s := range got {
		byName[s.Name] = s
	}

	if !byName["プロテイン"].Fits {
		t.Error("プロテイン(120kcal) は 200kcal に入るはず")
	}
	if byName["カツ丼"].Fits {
		t.Error("カツ丼(900kcal) は 200kcal に入らない")
	}
}

func TestSuggestFoods_残量が無ければ提案しない(t *testing.T) {
	t.Parallel()

	// 既に目標を超えている
	rem := analytics.Remaining{Kcal: -100, ProteinG: -5, FatG: -2, CarbG: -20}

	if got := analytics.SuggestFoods(rem, candidates(), 10); len(got) != 0 {
		t.Errorf("提案 = %d 件, want 0（食べる余地が無い）", len(got))
	}
}

func TestSuggestFoods_件数を絞る(t *testing.T) {
	t.Parallel()

	rem := analytics.Remaining{Kcal: 1000, ProteinG: 50, FatG: 30, CarbG: 100}

	if got := analytics.SuggestFoods(rem, candidates(), 2); len(got) != 2 {
		t.Errorf("提案 = %d 件, want 2", len(got))
	}
}

func TestSuggestFoods_PFCが無い候補は使わない(t *testing.T) {
	t.Parallel()

	rem := analytics.Remaining{Kcal: 600, ProteinG: 50, FatG: 20, CarbG: 60}
	cs := []analytics.FoodCandidate{{Name: "記録だけの食品", Count: 5}}

	// PFC が無いと「埋まるか」を判断できない
	if got := analytics.SuggestFoods(rem, cs, 10); len(got) != 0 {
		t.Errorf("提案 = %+v, want 0件", got)
	}
}

func TestSuggestFoods_同点なら食べている回数が多い方(t *testing.T) {
	t.Parallel()

	rem := analytics.Remaining{Kcal: 600, ProteinG: 50, FatG: 20, CarbG: 60}
	cs := []analytics.FoodCandidate{
		{Name: "たまに食べる", Count: 2, Kcal: 120, ProteinG: 24, FatG: 1.5, CarbG: 2},
		{Name: "よく食べる", Count: 40, Kcal: 120, ProteinG: 24, FatG: 1.5, CarbG: 2},
	}

	got := analytics.SuggestFoods(rem, cs, 2)
	if got[0].Name != "よく食べる" {
		t.Errorf("先頭 = %q, want よく食べる", got[0].Name)
	}
}

func TestSuggestFoods_同じタンパク質なら安い方(t *testing.T) {
	t.Parallel()

	// **制約はカロリー。** 同じだけタンパク質が取れるなら、
	// カロリー予算を使わない方がよい
	rem := analytics.Remaining{Kcal: 2000, ProteinG: 170, FatG: 60, CarbG: 200}
	cs := []analytics.FoodCandidate{
		{Name: "カツ丼", Count: 2, Kcal: 900, ProteinG: 30, FatG: 30, CarbG: 120},
		{Name: "サラダチキン", Count: 20, Kcal: 114, ProteinG: 24.1, FatG: 1.5, CarbG: 0.3},
	}

	got := analytics.SuggestFoods(rem, cs, 2)
	if got[0].Name != "サラダチキン" {
		t.Errorf("先頭 = %q, want サラダチキン（P は少し少ないがカロリーが1/8）", got[0].Name)
	}
}
