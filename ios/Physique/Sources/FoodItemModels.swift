import Foundation

/// 食品マスタの1項目（要件 N-02 / ADR-0017）。
///
/// **本体の PFC は合計。引数はそのうちの一部を担う**（#224）。
///
///     固定部 = 本体PFC − Σ 引数PFC
///     total  = 固定部 + Σ 引数PFC × (入力量 / 登録時の量)
struct FoodItem: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    var name: String
    var qty: String?

    /// **合計。** 引数はこの内訳で、超えてはいけない（#224）
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?

    var components: [FoodItemComponent]
    /// 選ばれた回数。一覧の並び順に使う
    var usedCount: Int?

    /// 引数を持つか。画面の出し分けに使う
    var hasComponents: Bool { !components.isEmpty }

    /// 選んだときに量を聞く必要があるか
    var needsAmount: Bool { hasComponents }

    /// 「全部」の引数（あれば）。画面の出し分けに使う
    var coversAllComponent: FoodItemComponent? {
        components.first { $0.coversAll ?? false }
    }

    /// 引数の初期値。登録したときの量をそのまま使う
    var defaultAmounts: [String: Double] {
        Dictionary(uniqueKeysWithValues: components.map { ($0.name, $0.amount) })
    }

    /// 入力量から PFC を出す。
    ///
    /// **サーバの `internal/foodmaster.Expand` と同じ式にする。**
    /// 食い違うと、画面の値が記録後に変わって見える。
    ///
    /// 渡さなかった引数は登録時の量。0 を渡したら 0（「今日は入れなかった」）。
    func expand(_ amounts: [String: Double] = [:]) -> Macros {
        // 固定部 = 合計 − 引数の登録時の分。**負にしない**
        var p = max((proteinG ?? 0) - components.reduce(0) { $0 + $1.proteinG }, 0)
        var f = max((fatG ?? 0) - components.reduce(0) { $0 + $1.fatG }, 0)
        var c = max((carbG ?? 0) - components.reduce(0) { $0 + $1.carbG }, 0)

        for comp in components {
            // **0 では割れない。** サーバ側の check で防いでいるが、
            // 古い端末から来た値で落ちないようにする。動かせないので登録どおり
            let ratio = comp.amount > 0
                ? (amounts[comp.name] ?? comp.amount) / comp.amount
                : 1

            p += comp.proteinG * ratio
            f += comp.fatG * ratio
            c += comp.carbG * ratio
        }

        return Macros(kcal: kcalFrom(p, f, c), proteinG: p, fatG: f, carbG: c)
    }

    /// Atwater 係数 4/9/4。サーバの `analytics.KcalFromMacros` と揃える
    private func kcalFrom(_ p: Double, _ f: Double, _ c: Double) -> Int {
        Int((p * 4 + f * 9 + c * 4).rounded())
    }
}

/// 引数1つ。**合計のうちこの引数が担う分**を、登録したときの量とともに持つ。
///
/// 量は1つだけ（#224）。「あたり」と「いつもの量」を別に持つと区別がつかない。
struct FoodItemComponent: Codable, Hashable, Sendable, Identifiable {
    var id: String { name }

    var name: String
    /// 表示専用。計算に使うのは比だけ
    var unit: String
    /// 登録したときの量。**基準にも初期値にもなる**
    var amount: Double

    var proteinG: Double
    var fatG: Double
    var carbG: Double

    /// 合計そのものを表すか。**立つと引数は1つだけ**
    var coversAll: Bool? = nil
}

/// 登録・更新で送る内容。
struct FoodItemInput: Codable, Sendable {
    var name: String
    var qty: String?
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?
    /// 省くか空なら引数なし
    var components: [FoodItemComponent]?
}

/// 小数点以下が無ければ整数で出す。
///
/// **入力欄に「30.0」と出ると打ち直しづらい。** 表示にも使う。
func numberText(_ v: Double) -> String {
    v == v.rounded() ? String(Int(v)) : String(v)
}

/// 引数を登録するときの入力。
///
/// **数値は文字列のまま持つ。** 数値に直しながら持つと「30.」で丸められて
/// 小数が打てない（MealDraft と同じ理由）。
struct FoodComponentDraft: Identifiable, Hashable, Sendable {
    let id = UUID()

    var name = ""
    var unit = "g"
    /// 登録したときの量。**基準にも初期値にもなる**（#224）
    var amount = ""
    var proteinG = ""
    var fatG = ""
    var carbG = ""
    /// 合計そのものを表すか。**立つと引数は1つだけ**
    var coversAll = false

    /// 送れる形にする。**読めなければ nil**
    func toComponent() -> FoodItemComponent? {
        let n = name.trimmingCharacters(in: .whitespaces)
        guard !n.isEmpty, let a = positive(amount) else { return nil }

        return FoodItemComponent(
            name: n, unit: unit.isEmpty ? "g" : unit,
            amount: a,
            proteinG: number(proteinG) ?? 0,
            fatG: number(fatG) ?? 0,
            carbG: number(carbG) ?? 0,
            coversAll: coversAll
        )
    }

    /// 何が足りないかを返す。**「登録できない」だけだと直しようがない**
    func problem() -> String {
        if name.trimmingCharacters(in: .whitespaces).isEmpty { return "引数の名前を入れる" }
        if positive(amount) == nil { return "量は 0 より大きい数にする" }

        return "引数の入力を見直す"
    }

    private func number(_ s: String) -> Double? {
        let t = s.trimmingCharacters(in: .whitespaces)

        return t.isEmpty ? nil : Double(t)
    }

    private func positive(_ s: String) -> Double? {
        guard let v = number(s), v > 0 else { return nil }

        return v
    }
}

// **拡張に置く。** 型の本体に init を書くと既定の init が消え、
// `FoodComponentDraft()`（引数を1つ足すとき）が使えなくなる
extension FoodComponentDraft {
    /// 登録済みの引数を直すときの初期値。
    init(_ c: FoodItemComponent) {
        self.init()
        name = c.name
        unit = c.unit
        amount = numberText(c.amount)
        proteinG = numberText(c.proteinG)
        fatG = numberText(c.fatG)
        carbG = numberText(c.carbG)
        coversAll = c.coversAll ?? false
    }
}
