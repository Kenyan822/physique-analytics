import Foundation

/// 食品マスタの1項目（要件 N-02 / ADR-0017）。
///
/// **引数は任意。** `components` が空なら `proteinG` 等をそのまま使う。
/// 量が変わるもの（プロテイン・料理の材料）だけ引数を持つ。
struct FoodItem: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    var name: String
    var qty: String?

    /// 引数が無いときに使う値。**`components` があるときは見ない**
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?

    var components: [FoodItemComponent]
    /// 選ばれた回数。一覧の並び順に使う
    var usedCount: Int?

    /// 引数を持つか。画面の出し分けに使う
    var hasComponents: Bool { !components.isEmpty }

    /// 引数の既定値。入力欄の初期表示に使う
    var defaultAmounts: [String: Double] {
        Dictionary(uniqueKeysWithValues: components.map { ($0.name, $0.defaultAmount) })
    }

    /// 入力量から PFC を出す。
    ///
    /// **サーバの `internal/foodmaster.Expand` と同じ式にする。**
    /// 食い違うと、画面の値が記録後に変わって見える。
    ///
    /// 渡さなかった引数は既定値。0 を渡したら 0（「今日は入れなかった」）。
    func expand(_ amounts: [String: Double]) -> Macros {
        guard hasComponents else {
            return Macros(
                kcal: kcalFrom(proteinG ?? 0, fatG ?? 0, carbG ?? 0),
                proteinG: proteinG ?? 0, fatG: fatG ?? 0, carbG: carbG ?? 0
            )
        }

        var p = 0.0, f = 0.0, c = 0.0
        for comp in components {
            // **0 では割れない。** サーバ側の check で防いでいるが、
            // 古い端末から来た値で落ちないようにする
            guard comp.basisAmount > 0 else { continue }

            let ratio = (amounts[comp.name] ?? comp.defaultAmount) / comp.basisAmount
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

/// 引数1つ。**基準量あたりの PFC** を持つ。
struct FoodItemComponent: Codable, Hashable, Sendable, Identifiable {
    var id: String { name }

    var name: String
    /// 表示専用。計算に使うのは比だけ
    var unit: String
    /// パッケージの「n g あたり」の n
    var basisAmount: Double
    /// 入力時の初期値
    var defaultAmount: Double

    var proteinG: Double
    var fatG: Double
    var carbG: Double
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

/// 引数を登録するときの入力。
///
/// **数値は文字列のまま持つ。** 数値に直しながら持つと「30.」で丸められて
/// 小数が打てない（MealDraft と同じ理由）。
struct FoodComponentDraft: Identifiable, Hashable, Sendable {
    let id = UUID()

    var name = ""
    var unit = "g"
    /// パッケージの表示は「100g あたり」が多いので既定にする
    var basisAmount = "100"
    var defaultAmount = ""
    var proteinG = ""
    var fatG = ""
    var carbG = ""

    /// 送れる形にする。**読めなければ nil**
    func toComponent() -> FoodItemComponent? {
        let n = name.trimmingCharacters(in: .whitespaces)
        guard !n.isEmpty, let basis = positive(basisAmount) else { return nil }

        // 既定値を書かなければ基準量と同じ。「30g あたり」を 30g 使う、が普通
        let def = number(defaultAmount) ?? basis

        return FoodItemComponent(
            name: n, unit: unit.isEmpty ? "g" : unit,
            basisAmount: basis, defaultAmount: def,
            proteinG: number(proteinG) ?? 0,
            fatG: number(fatG) ?? 0,
            carbG: number(carbG) ?? 0
        )
    }

    /// 何が足りないかを返す。**「登録できない」だけだと直しようがない**
    func problem() -> String {
        if name.trimmingCharacters(in: .whitespaces).isEmpty { return "引数の名前を入れる" }
        if positive(basisAmount) == nil { return "基準量は 0 より大きい数にする" }

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
