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

	got := foodmaster.Expand(item, nil)

	assertMacros(t, got, 24, 1.5, 2)
}

func TestExpand_引数が無く値も無ければ0(t *testing.T) {
	t.Parallel()

	got := foodmaster.Expand(foodmaster.Item{}, nil)

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

	got := foodmaster.Expand(item, map[string]float64{"量": 45})

	assertMacros(t, got, 36, 2.25, 3)
}

func TestExpand_渡さなければ既定値を使う(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: 24, FatG: 1.5, CarbG: 2},
		},
	}

	got := foodmaster.Expand(item, nil)

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

	got := foodmaster.Expand(item, map[string]float64{"鶏ひき肉": 230})

	// 肉 2.3 倍 + 砂糖は既定の 10g
	assertMacros(t, got, 17.5*2.3, 12.0*2.3, 99.2*0.1)
}

func TestExpand_構成があれば項目のPFCは見ない(t *testing.T) {
	t.Parallel()

	// **二重に足さない。** 項目の値は「引数が無いとき」だけのもの
	item := foodmaster.Item{
		ProteinG: ptr(999.0),
		Components: []foodmaster.Component{
			{Name: "量", BasisAmount: 10, DefaultAmount: 10, ProteinG: 5},
		},
	}

	got := foodmaster.Expand(item, nil)

	assertMacros(t, got, 5, 0, 0)
}

func TestExpand_0を渡せる(t *testing.T) {
	t.Parallel()

	// 「今日は砂糖を入れなかった」。**既定値に戻さない**
	item := foodmaster.Item{
		Components: []foodmaster.Component{
			{Name: "砂糖", BasisAmount: 100, DefaultAmount: 10, CarbG: 99.2},
		},
	}

	got := foodmaster.Expand(item, map[string]float64{"砂糖": 0})

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

	got := foodmaster.Expand(item, map[string]float64{"存在しない": 100})

	assertMacros(t, got, 5, 0, 0)
}

func TestExpand_基準量が0なら0扱い(t *testing.T) {
	t.Parallel()

	// DB の check で防いでいるが、**割り算の前に落ちないことを保証する**
	item := foodmaster.Item{
		Components: []foodmaster.Component{{Name: "量", BasisAmount: 0, DefaultAmount: 10, ProteinG: 5}},
	}

	got := foodmaster.Expand(item, nil)

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
