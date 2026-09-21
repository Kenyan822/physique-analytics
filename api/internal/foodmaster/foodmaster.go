// Package foodmaster は食品マスタを PFC に展開する（要件 N-02 / ADR-0017）。
//
// **本体の PFC と引数は足し算**（#218）。
//
//	total = 本体PFC × (ScalesWithAmount ? 入力量 / BaseAmount : 1)
//	      + Σ 引数PFC × (引数量 / 引数基準量)
//
// 大半の食品は量が固定なので、基本は登録した PFC をそのまま返す。
// 全量が1つの量で決まるもの（プロテイン）は ScalesWithAmount を立て、
// 固定部分と変わる部分があるもの（料理）は引数を持つ。
package foodmaster

// Macros は展開した結果。
//
// **kcal を持たない。** PFC から出せる（Atwater 4/9/4）ので、
// 持つと手入力と計算値が食い違ったときにどちらが正か決められなくなる。
type Macros struct {
	ProteinG float64
	FatG     float64
	CarbG    float64
}

// Item は食品マスタの1項目。
type Item struct {
	// ProteinG 等は本体の PFC。**引数があっても使う**（#218）。
	// nil は「登録していない」＝0
	ProteinG *float64
	FatG     *float64
	CarbG    *float64

	// BaseAmount は「n g あたり」の n。ScalesWithAmount のときだけ使う
	BaseAmount *float64
	// ScalesWithAmount は全量が1つの量で決まるか。
	// **立てると引数の行を作らずに比例させられる**（プロテイン）
	ScalesWithAmount bool

	// Components が空なら引数なし
	Components []Component
}

// Component は引数1つ。基準量あたりの PFC を持つ。
type Component struct {
	Name string
	// BasisAmount はパッケージの「n g あたり」の n
	BasisAmount float64
	// DefaultAmount は入力時の初期値。触らなければこれが使われる
	DefaultAmount float64

	ProteinG float64
	FatG     float64
	CarbG    float64
}

// Expand は入力量から PFC を出す。
//
// base は本体の入力量。ScalesWithAmount が立っているときだけ見る。
// nil なら基準量ぶん（＝登録したまま）。
//
// amounts は引数の名前 → 入力量。**渡さなかった引数は既定値**を使う。
// 0 を渡したら 0 として扱う（「今日は入れなかった」を表せるようにする）。
func Expand(item Item, base *float64, amounts map[string]float64) Macros {
	// **本体は常に足す**（#218）。引数があっても捨てない
	r := baseRatio(item, base)
	out := Macros{
		ProteinG: value(item.ProteinG) * r,
		FatG:     value(item.FatG) * r,
		CarbG:    value(item.CarbG) * r,
	}

	for _, c := range item.Components {
		// DB の check で防いでいるが、**割り算の前に落ちない**ことを保証する
		if c.BasisAmount <= 0 {
			continue
		}

		amount := c.DefaultAmount
		if v, ok := amounts[c.Name]; ok {
			amount = v
		}
		ratio := amount / c.BasisAmount

		out.ProteinG += c.ProteinG * ratio
		out.FatG += c.FatG * ratio
		out.CarbG += c.CarbG * ratio
	}

	return out
}

// baseRatio は本体にかける倍率を返す。
//
// **比例しないなら 1。** 基準量が無い・0 のときも 1 に倒す
// （DB の check で防いでいるが、割り算の前に落ちないことを保証する）。
func baseRatio(item Item, base *float64) float64 {
	if !item.ScalesWithAmount || item.BaseAmount == nil || *item.BaseAmount <= 0 {
		return 1
	}
	if base == nil {
		return 1
	}

	return *base / *item.BaseAmount
}

// Defaults は引数の既定値を返す。入力画面の初期表示に使う。
func Defaults(item Item) map[string]float64 {
	if len(item.Components) == 0 {
		return nil
	}

	out := make(map[string]float64, len(item.Components))
	for _, c := range item.Components {
		out[c.Name] = c.DefaultAmount
	}

	return out
}

func value(v *float64) float64 {
	if v == nil {
		return 0
	}

	return *v
}
