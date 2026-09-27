// Package foodmaster は食品マスタを PFC に展開する（要件 N-02 / ADR-0017）。
//
// **本体の PFC は「合計」。引数はそのうちの一部を担う**（#224）。
//
//	固定部 = 本体PFC − Σ 引数PFC
//	total  = 固定部 + Σ 引数PFC × (入力量 / 登録時の量)
//
// 触らなければ合計に戻る。量を変えた引数だけが動く。
//
// 大半の食品は量が固定なので、基本は登録した PFC をそのまま返す。
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
	// ProteinG 等は**合計**。引数はこの内訳（#224）。
	// nil は「登録していない」＝0
	ProteinG *float64
	FatG     *float64
	CarbG    *float64

	// Components が空なら引数なし
	Components []Component
}

// Component は引数1つ。
//
// **量は1つだけ持つ。** 「n g あたり」と「いつもの量」を別に持っていたが、
// 区別がつかないと言われたので統合した（#224）。
// 登録した量が基準にも初期値にもなる。
type Component struct {
	Name string
	// Amount は登録したときの量。**0 では割れない**
	Amount float64
	// ProteinG 等は Amount ぶんの値。合計のうちこの引数が担う分
	ProteinG float64
	FatG     float64
	CarbG    float64

	// CoversAll は合計そのものを表すか。**立つと引数は1つだけ**（プロテイン）
	CoversAll bool
}

// Expand は入力量から PFC を出す。
//
// amounts は引数の名前 → 入力量。**渡さなかった引数は登録時の量**を使う。
// 0 を渡したら 0 として扱う（「今日は入れなかった」を表せるようにする）。
func Expand(item Item, amounts map[string]float64) Macros {
	// 固定部 = 合計 − 引数の登録時の分
	fixed := Macros{
		ProteinG: value(item.ProteinG),
		FatG:     value(item.FatG),
		CarbG:    value(item.CarbG),
	}
	for _, c := range item.Components {
		fixed.ProteinG -= c.ProteinG
		fixed.FatG -= c.FatG
		fixed.CarbG -= c.CarbG
	}

	// **負にしない。** Validate で防いでいるが、古いデータで
	// マイナスの食事が出ると読めなくなる
	out := Macros{
		ProteinG: atLeastZero(fixed.ProteinG),
		FatG:     atLeastZero(fixed.FatG),
		CarbG:    atLeastZero(fixed.CarbG),
	}

	for _, c := range item.Components {
		// DB の check で防いでいるが、**割り算の前に落ちない**ことを保証する。
		// 動かせないので登録どおりの分を足す
		ratio := 1.0
		if c.Amount > 0 {
			amount := c.Amount
			if v, ok := amounts[c.Name]; ok {
				amount = v
			}
			ratio = amount / c.Amount
		}

		out.ProteinG += c.ProteinG * ratio
		out.FatG += c.FatG * ratio
		out.CarbG += c.CarbG * ratio
	}

	return out
}

// Validate は登録できる形かを見る。**通れば空文字**。
//
// 引数は合計のうちの一部なので、**合計を超えてはいけない**（#224）。
func Validate(item Item) string {
	if len(item.Components) == 0 {
		return ""
	}

	for _, m := range []struct {
		name  string
		total float64
		part  func(Component) float64
	}{
		{"P", value(item.ProteinG), func(c Component) float64 { return c.ProteinG }},
		{"F", value(item.FatG), func(c Component) float64 { return c.FatG }},
		{"C", value(item.CarbG), func(c Component) float64 { return c.CarbG }},
	} {
		var sum float64
		for _, c := range item.Components {
			sum += m.part(c)
		}
		// 浮動小数の誤差で弾かない
		if sum > m.total+0.01 {
			return m.name + " の引数の合計が全体を超えている"
		}
	}

	return ""
}

// Defaults は引数の初期値を返す。入力画面の初期表示に使う。
func Defaults(item Item) map[string]float64 {
	if len(item.Components) == 0 {
		return nil
	}

	out := make(map[string]float64, len(item.Components))
	for _, c := range item.Components {
		out[c.Name] = c.Amount
	}

	return out
}

func value(v *float64) float64 {
	if v == nil {
		return 0
	}

	return *v
}

func atLeastZero(v float64) float64 {
	if v < 0 {
		return 0
	}

	return v
}
