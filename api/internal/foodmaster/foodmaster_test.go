package foodmaster_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/foodmaster"
)

func TestExpand_引数が無ければそのまま返す(t *testing.T) {
	t.Parallel()

	// **基本は引数なし**（ADR-0017）。登録した PFC をそのまま使う
	item := foodmaster.Item{ProteinG: ptr(24.0), FatG: ptr(1.5), CarbG: ptr(2.0)}

	got := foodmaster.Expand(item, nil, nil)

	assertMacros(t, got, 24, 1.5, 2)
}

func TestExpand_引数が無く値も無ければ0(t *testing.T) {
	t.Parallel()

	got := foodmaster.Expand(foodmaster.Item{}, nil, nil)

	assertMacros(t, got, 0, 0, 0)
}

func TestExpand_基準量との比で計算する(t *testing.T) {
	t.Parallel()

	// 30g あたり P24 F1.5 C2 を 45g ぶん
	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: 24, FatG: 1.5, CarbG: 2},
		},
	}

	got := foodmaster.Expand(item, nil, map[string]float64{"量": 45})

	assertMacros(t, got, 36, 2.25, 3)
}

func TestExpand_渡さなければ既定値を使う(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: 24, FatG: 1.5, CarbG: 2},
		},
	}

	got := foodmaster.Expand(item, nil, nil)

	assertMacros(t, got, 24, 1.5, 2)
}

func TestExpand_引数を複数持てる(t *testing.T) {
	t.Parallel()

	// 料理。**買った肉の量だけ変える**
	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "鶏ひき肉", BasisAmount: 100, DefaultAmount: 200, ProteinG: 17.5, FatG: 12.0, CarbG: 0},
			{Name: "砂糖", BasisAmount: 100, DefaultAmount: 10, ProteinG: 0, FatG: 0, CarbG: 99.2},
		},
	}

	got := foodmaster.Expand(item, nil, map[string]float64{"鶏ひき肉": 230})

	// 肉 2.3 倍 + 砂糖は既定の 10g
	assertMacros(t, got, 17.5*2.3, 12.0*2.3, 99.2*0.1)
}

func TestExpand_本体と引数を足す(t *testing.T) {
	t.Parallel()

	// **本体は常に効く**（#218 で方針を変えた）。定食の固定部分＋量が変わる肉、
	// のような形を1項目で書けるようにするため
	item := foodmaster.Item{
		ProteinG: ptr(20.0),
		Components: []foodmaster.Component{
			{Name: "鶏むね", BasisAmount: 100, DefaultAmount: 200, ProteinG: 23},
		},
	}

	got := foodmaster.Expand(item, nil, nil)

	assertMacros(t, got, 20+46, 0, 0)
}

func TestExpand_0を渡せる(t *testing.T) {
	t.Parallel()

	// 「今日は砂糖を入れなかった」。**既定値に戻さない**
	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "砂糖", BasisAmount: 100, DefaultAmount: 10, CarbG: 99.2},
		},
	}

	got := foodmaster.Expand(item, nil, map[string]float64{"砂糖": 0})

	assertMacros(t, got, 0, 0, 0)
}

func TestExpand_知らない引数は無視する(t *testing.T) {
	t.Parallel()

	// 構成を消したあとに古い入力が飛んできても落とさない
	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "量", BasisAmount: 10, DefaultAmount: 10, ProteinG: 5},
		},
	}

	got := foodmaster.Expand(item, nil, map[string]float64{"存在しない": 100})

	assertMacros(t, got, 5, 0, 0)
}

func TestExpand_基準量が0なら0扱い(t *testing.T) {
	t.Parallel()

	// DB の check で防いでいるが、**割り算の前に落ちないことを保証する**
	item := foodmaster.Item{
		Components: []foodmaster.Component{{Name: "量", BasisAmount: 0, DefaultAmount: 10, ProteinG: 5}},
	}

	got := foodmaster.Expand(item, nil, nil)

	assertMacros(t, got, 0, 0, 0)
}

func assertMacros(t *testing.T, got foodmaster.Macros, p, f, c float64) {
	t.Helper()

	for _, x := range []struct {
		name      string
		got, want float64
	}{
		{"ProteinG", got.ProteinG, p},
		{"FatG", got.FatG, f},
		{"CarbG", got.CarbG, c},
	} {
		if math.Abs(x.got-x.want) > 0.001 {
			t.Errorf("%s = %v, want %v", x.name, x.got, x.want)
		}
	}
}

func ptr[T any](v T) *T { return &v }

// ---- #218: 本体の比例と、既存データが変わらないこと ----

func TestExpand_全量が量に比例する(t *testing.T) {
	t.Parallel()

	// プロテイン。**引数の行を作らずに済ませる**（#218）
	item := foodmaster.Item{
		ProteinG: ptr(24.0), FatG: ptr(1.5), CarbG: ptr(2.0),
		BaseAmount: ptr(30.0), ScalesWithAmount: true,
	}

	got := foodmaster.Expand(item, ptr(45.0), nil)

	// 24 × 1.5 = 36 / 1.5 × 1.5 = 2.25 / 2 × 1.5 = 3
	assertMacros(t, got, 36, 2.25, 3)
}

func TestExpand_比例しないなら入力量を無視する(t *testing.T) {
	t.Parallel()

	// チェックが無ければ基準量も入力量も使わない
	item := foodmaster.Item{ProteinG: ptr(24.0), BaseAmount: ptr(30.0)}

	got := foodmaster.Expand(item, ptr(999.0), nil)

	assertMacros(t, got, 24, 0, 0)
}

func TestExpand_比例するのに基準量が無ければそのまま(t *testing.T) {
	t.Parallel()

	// DB の check で防いでいるが、**割り算の前に落ちない**ことを保証する
	item := foodmaster.Item{ProteinG: ptr(24.0), ScalesWithAmount: true}

	got := foodmaster.Expand(item, ptr(45.0), nil)

	assertMacros(t, got, 24, 0, 0)
}

func TestExpand_比例するが量を渡さなければ基準量ぶん(t *testing.T) {
	t.Parallel()

	// 量を聞かずに確定した場合。**パッケージ通りの1食分**が入る
	item := foodmaster.Item{
		ProteinG: ptr(24.0), BaseAmount: ptr(30.0), ScalesWithAmount: true,
	}

	got := foodmaster.Expand(item, nil, nil)

	assertMacros(t, got, 24, 0, 0)
}

func TestExpand_比例と引数を同時に持てる(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{
		ProteinG: ptr(24.0), BaseAmount: ptr(30.0), ScalesWithAmount: true,
		Components: []foodmaster.Component{
			{Name: "牛乳", BasisAmount: 100, DefaultAmount: 200, ProteinG: 3.3},
		},
	}

	got := foodmaster.Expand(item, ptr(45.0), nil)

	// 36 + 6.6
	assertMacros(t, got, 42.6, 0, 0)
}

// **既存データの結果が変わらないことを固定する。**
// これが崩れると、いままで記録してきた値と食い違う（#218 の一番の関心事）
func TestExpand_既存の登録は結果が変わらない(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		item  foodmaster.Item
		wantP float64
	}{
		{
			"引数なし（本体PFCのみ）",
			foodmaster.Item{ProteinG: ptr(6.5)},
			6.5,
		},
		{
			"引数あり（本体は未登録）",
			foodmaster.Item{Components: []foodmaster.Component{
				{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: 24},
			}},
			24,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// 新しい列は既定値のまま（比例しない・基準量なし）
			got := foodmaster.Expand(tt.item, nil, nil)

			assertMacros(t, got, tt.wantP, 0, 0)
		})
	}
}
