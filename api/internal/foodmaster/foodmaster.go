// Package foodmaster は食品マスタを PFC に展開する（要件 N-02 / ADR-0017）。
//
// **引数は任意。** 大半の食品は量が固定なので、基本は登録した PFC を
// そのまま返す。量が変わるもの（プロテイン・料理の材料）だけ引数を持つ。
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
	// ProteinG 等は**引数が無いときだけ**使う。nil は「登録していない」
	ProteinG *float64
	FatG     *float64
	CarbG    *float64

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
// amounts は引数の名前 → 入力量。**渡さなかった引数は既定値**を使う。
// 0 を渡したら 0 として扱う（「今日は入れなかった」を表せるようにする）。
func Expand(item Item, amounts map[string]float64) Macros {
	// **引数が無ければ登録した値をそのまま返す。** これが基本の経路（ADR-0017）
	if len(item.Components) == 0 {
		return Macros{
			ProteinG: value(item.ProteinG),
			FatG:     value(item.FatG),
			CarbG:    value(item.CarbG),
		}
	}

	var out Macros
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
