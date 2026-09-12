package analytics

import (
	"math"
	"slices"
)

// 不足分を満たす食品の提案（要件 N-07）。
//
// **食品マスタは持たない**（docs/01-要件定義.md §4.3）ので、候補は
// 自分の記録履歴から来る。知らない食品は提案しない。

// Remaining はその日の残量（目標 − 実績）。
type Remaining struct {
	Kcal     float64
	ProteinG float64
	FatG     float64
	CarbG    float64
}

// FoodCandidate は履歴から作った候補。
type FoodCandidate struct {
	Name string
	// Count は記録した回数。同点のときの並びに使う
	Count    int
	Kcal     float64
	ProteinG float64
	FatG     float64
	CarbG    float64
}

// FoodSuggestion は提案1件。
type FoodSuggestion struct {
	Name string
	// Fits は残りのカロリーに収まるか
	Fits bool
	// Score は「不足をどれだけ埋めるか」。並べ替えに使う
	Score float64
	// FillsProteinPct はタンパク質の不足を何%埋めるか
	FillsProteinPct float64
}

// SuggestFoods は残量を埋める候補を順に返す。
//
// **タンパク質を優先する。** 減量期に一番守りたいのが LBM で、
// 足りないと分かっていても最後に回されがち。カロリーを超える候補は
// 順位を下げるが、消しはしない（「これを食べるなら他を削る」は成立する）。
func SuggestFoods(rem Remaining, candidates []FoodCandidate, limit int) []FoodSuggestion {
	// 目標を超えていれば食べる余地が無い
	if rem.Kcal <= 0 {
		return nil
	}

	out := make([]FoodSuggestion, 0, len(candidates))
	for _, c := range candidates {
		// PFC が無いと「埋まるか」を判断できない
		if c.Kcal <= 0 && c.ProteinG <= 0 {
			continue
		}

		fills := 0.0
		if rem.ProteinG > 0 {
			fills = math.Min(1, c.ProteinG/rem.ProteinG)
		}

		// **カロリー予算をどれだけ使うかで割り引く。** 制約はカロリーなので、
		// 同じタンパク質なら安く済む方がよい。カツ丼（900kcal で P30g）より
		// サラダチキン（114kcal で P24g）が上に来る。
		// 1 を超える＝予算に入らない候補は、これで大きく沈む
		cost := c.Kcal / rem.Kcal

		out = append(out, FoodSuggestion{
			Name:            c.Name,
			Fits:            c.Kcal <= rem.Kcal,
			Score:           fills - cost,
			FillsProteinPct: fills * 100,
		})
	}

	byCount := make(map[string]int, len(candidates))
	for _, c := range candidates {
		byCount[c.Name] = c.Count
	}

	slices.SortStableFunc(out, func(a, b FoodSuggestion) int {
		if a.Score != b.Score {
			// 高い方が先
			if a.Score > b.Score {
				return -1
			}

			return 1
		}

		// 同点なら食べている回数が多い方。実際に手が伸びるものを上に出す
		return byCount[b.Name] - byCount[a.Name]
	})

	if limit > 0 && len(out) > limit {
		out = out[:limit]
	}

	return out
}
