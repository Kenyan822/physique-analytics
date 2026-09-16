import Foundation
import Observation

/// 入力中の1件。**数値は文字列のまま持つ。**
/// 数値に直しながら持つと「62.」と打った時点で 62 に丸められ、続きが打てない
struct MealDraft: Sendable, Equatable {
    var name = ""
    var qty = ""
    var kcal = ""
    var proteinG = ""
    var fatG = ""
    var carbG = ""

    var isEmpty: Bool { name.trimmingCharacters(in: .whitespaces).isEmpty }

    func toInput(date: String, slot: MealSlot) -> MealInput {
        MealInput(
            id: UUID(),
            date: date,
            slot: slot,
            name: name.trimmingCharacters(in: .whitespaces),
            qty: text(qty),
            kcal: int(kcal),
            proteinG: number(proteinG),
            fatG: number(fatG),
            carbG: number(carbG),
            source: .manual
        )
    }

    /// **空欄は 0 ではなく未入力。** 0 として送ると
    /// 「食べたが記録が雑」と「0 kcal」が同じになる
    private func number(_ s: String) -> Double? {
        let t = s.trimmingCharacters(in: .whitespaces)

        return t.isEmpty ? nil : Double(t)
    }

    private func int(_ s: String) -> Int? {
        number(s).map { Int($0) }
    }

    private func text(_ s: String) -> String? {
        let t = s.trimmingCharacters(in: .whitespaces)

        return t.isEmpty ? nil : t
    }
}

/// 食事の記録（要件 N-01 / N-02 / N-05）。
@Observable
@MainActor
final class MealModel {
    private(set) var meals: [Meal] = []
    private(set) var suggestions: [MealSuggestion] = []
    private(set) var target: Macros?
    /// 目標を出せない理由。**出せなくても記録は続けられる**
    private(set) var targetsMessage: String?
    private(set) var errorMessage: String?
    private(set) var isWorking = false

    var draft = MealDraft()
    var slot: MealSlot

    let date: String
    private let api: APIClient

    var totals: MealTotals { MealTotals(of: meals) }
    var remaining: Macros? { target.map { totals.remaining(from: $0) } }

    init(api: APIClient, date: String = JST.dateString(), now: Date = Date()) {
        self.api = api
        self.date = date
        self.slot = MealSlot.suggested(at: now)
    }

    func load() async {
        isWorking = true
        defer { isWorking = false }

        do {
            meals = try await api.listMeals(from: date, to: date)
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "読み込めない"

            return
        }

        // **目標は取れなくてもよい。** フェーズ未登録だと 422 になるが、
        // ここで止めると記録そのものができなくなる
        do {
            let t = try await api.dailyTargets(date: date)
            target = t.target
            targetsMessage = t.target == nil ? t.note : nil
        } catch let e as APIError {
            target = nil
            targetsMessage = e.errorDescription
        } catch {
            target = nil
        }
    }

    /// 過去の記録から候補を引く（要件 N-02）。
    func loadSuggestions(query: String) async {
        suggestions = (try? await api.mealSuggestions(query: query)) ?? []
    }

    /// 候補を選ぶ。**選んだ時点で入力が終わる**のが狙い
    func pick(_ s: MealSuggestion) {
        draft.name = s.name
        draft.qty = s.qty ?? ""
        draft.kcal = s.kcal.map(String.init) ?? ""
        draft.proteinG = s.proteinG.map { trim($0) } ?? ""
        draft.fatG = s.fatG.map { trim($0) } ?? ""
        draft.carbG = s.carbG.map { trim($0) } ?? ""
        suggestions = []
    }

    func record() async {
        guard !draft.isEmpty else { return }
        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        do {
            let created = try await api.createMeal(draft.toInput(date: date, slot: slot))
            meals.append(created)
            // 続けて入れられるよう空に戻す。区分は据え置き
            draft = MealDraft()
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "記録できない"
        }
    }

    func delete(_ meal: Meal) async {
        do {
            try await api.deleteMeal(id: meal.id)
            meals.removeAll { $0.id == meal.id }
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "消せない"
        }
    }

    /// 62.0 を "62" にする。末尾の .0 は入力欄で邪魔
    private func trim(_ v: Double) -> String {
        v == v.rounded() ? String(Int(v)) : String(v)
    }
}
