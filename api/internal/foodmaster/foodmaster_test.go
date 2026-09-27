package foodmaster_test

import (
	"math"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/foodmaster"
)

// 鶏そぼろ定食。**合計 P66 のうち 46 を鶏むねが占める**
func teishoku() foodmaster.Item {
	return foodmaster.Item{
		ProteinG: ptr(66.0), FatG: ptr(13.8), CarbG: ptr(80.0),
		Components: []foodmaster.Component{
			{Name: "鶏むね", Amount: 200, ProteinG: 46, FatG: 3.8, CarbG: 0},
		},
	}
}

// プロテイン。**全部が1つの引数で決まる**
func protein() foodmaster.Item {
	return foodmaster.Item{
		ProteinG: ptr(24.0), FatG: ptr(1.5), CarbG: ptr(2.0),
		Components: []foodmaster.Component{
			{Name: "量", Amount: 30, ProteinG: 24, FatG: 1.5, CarbG: 2, CoversAll: true},
		},
	}
}

func TestExpand_引数が無ければそのまま返す(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{ProteinG: ptr(6.5), FatG: ptr(5.2), CarbG: ptr(0.2)}

	assertMacros(t, foodmaster.Expand(item, nil), 6.5, 5.2, 0.2)
}

func TestExpand_引数が無く値も無ければ0(t *testing.T) {
	t.Parallel()

	assertMacros(t, foodmaster.Expand(foodmaster.Item{}, nil), 0, 0, 0)
}

// **触らなければ登録したときの合計に戻る。** ここが差し引きの肝
func TestExpand_触らなければ合計になる(t *testing.T) {
	t.Parallel()

	assertMacros(t, foodmaster.Expand(teishoku(), nil), 66, 13.8, 80)
}

func TestExpand_引数を変えるとその分だけ動く(t *testing.T) {
	t.Parallel()

	// 固定部 20 + 46 × 230/200 = 72.9
	got := foodmaster.Expand(teishoku(), map[string]float64{"鶏むね": 230})

	assertMacros(t, got, 72.9, 13.8-3.8+3.8*1.15, 80)
}

func TestExpand_引数を0にすると固定部だけ残る(t *testing.T) {
	t.Parallel()

	// 「今日は肉を入れなかった」。ごはんと味噌汁だけ
	got := foodmaster.Expand(teishoku(), map[string]float64{"鶏むね": 0})

	assertMacros(t, got, 20, 10, 80)
}

func TestExpand_全部なら固定部が0(t *testing.T) {
	t.Parallel()

	assertMacros(t, foodmaster.Expand(protein(), map[string]float64{"量": 45}), 36, 2.25, 3)
}

func TestExpand_全部でも触らなければ登録どおり(t *testing.T) {
	t.Parallel()

	assertMacros(t, foodmaster.Expand(protein(), nil), 24, 1.5, 2)
}

func TestExpand_知らない引数は無視する(t *testing.T) {
	t.Parallel()

	got := foodmaster.Expand(teishoku(), map[string]float64{"存在しない": 999})

	assertMacros(t, got, 66, 13.8, 80)
}

func TestExpand_登録時の量が0なら動かさない(t *testing.T) {
	t.Parallel()

	// DB の check で防いでいるが、**割り算の前に落ちない**ことを保証する
	item := foodmaster.Item{
		ProteinG:   ptr(10.0),
		Components: []foodmaster.Component{{Name: "x", Amount: 0, ProteinG: 5}},
	}

	assertMacros(t, foodmaster.Expand(item, map[string]float64{"x": 100}), 10, 0, 0)
}

// **固定部が負にならない。** バリデーションで防ぐが、古いデータで落ちないように
func TestExpand_引数が合計を超えても負にしない(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{
		ProteinG:   ptr(10.0),
		Components: []foodmaster.Component{{Name: "x", Amount: 100, ProteinG: 30}},
	}

	got := foodmaster.Expand(item, map[string]float64{"x": 100})

	// 固定部は 0 に丸める。30 がそのまま出る
	assertMacros(t, got, 30, 0, 0)
}

func TestDefaults_登録時の量を返す(t *testing.T) {
	t.Parallel()

	got := foodmaster.Defaults(teishoku())

	if got["鶏むね"] != 200 {
		t.Errorf("Defaults[鶏むね] = %v, want 200", got["鶏むね"])
	}
}

// ---- バリデーション（#224）----

func TestValidate_引数が合計を超えたら弾く(t *testing.T) {
	t.Parallel()

	item := foodmaster.Item{
		ProteinG:   ptr(10.0),
		Components: []foodmaster.Component{{Name: "肉", Amount: 100, ProteinG: 30}},
	}

	if msg := foodmaster.Validate(item); msg == "" {
		t.Error("超えているのに通った")
	}
}

func TestValidate_合計と同じなら通る(t *testing.T) {
	t.Parallel()

	if msg := foodmaster.Validate(protein()); msg != "" {
		t.Errorf("Validate = %q, want 空", msg)
	}
}

func TestValidate_収まっていれば通る(t *testing.T) {
	t.Parallel()

	if msg := foodmaster.Validate(teishoku()); msg != "" {
		t.Errorf("Validate = %q, want 空", msg)
	}
}

func TestValidate_PFCごとに見る(t *testing.T) {
	t.Parallel()

	// P は収まっているが F が超えている
	item := foodmaster.Item{
		ProteinG: ptr(50.0), FatG: ptr(1.0),
		Components: []foodmaster.Component{{Name: "肉", Amount: 100, ProteinG: 10, FatG: 20}},
	}

	if msg := foodmaster.Validate(item); msg == "" {
		t.Error("F が超えているのに通った")
	}
}

func TestValidate_引数が無ければ通る(t *testing.T) {
	t.Parallel()

	if msg := foodmaster.Validate(foodmaster.Item{ProteinG: ptr(6.5)}); msg != "" {
		t.Errorf("Validate = %q, want 空", msg)
	}
}

func ptr(v float64) *float64 { return &v }

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
		if math.Abs(x.got-x.want) > 0.01 {
			t.Errorf("%s = %v, want %v", x.name, x.got, x.want)
		}
	}
}
