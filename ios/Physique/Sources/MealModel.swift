import Foundation
import Observation

/// 入力中の1件。**数値は文字列のまま持つ。**
/// 数値に直しながら持つと「62.」と打った時点で 62 に丸められ、続きが打てない
struct MealDraft: Sendable, Equatable {
    // **PFC を先頭に置く。** 入力の主目的がこれで、名前と量は後回しでよい（#188）
    var proteinG = ""
    var fatG = ""
    var carbG = ""

    var name = ""
    var qty = ""

    /// **PFC が1つでも入っていれば記録できる。** 名前は任意
    var isEmpty: Bool {
        [proteinG, fatG, carbG, name].allSatisfy {
            $0.trimmingCharacters(in: .whitespaces).isEmpty
        }
    }

    /// 表示用の kcal（Atwater 4/9/4）。
    ///
    /// **送らない。** サーバが同じ式で計算する（`analytics.KcalFromMacros`）。
    /// 二重に持つと、どちらが正かの判断が要る。ここは打ちながら見えると
    /// 便利、というだけの値
    /// **空欄は 0 とみなす。** 鶏むねの C のように「本当に 0」で空のまま
    /// 済ませる場面が普通にある。全部揃うまで出さないと、爆速入力のつもりが
    /// 「なぜ出ないのか」を考える時間になる。
    ///
    /// ただし**1つも入っていなければ nil**。名前だけの記録に 0 kcal を付けない
    var kcal: Int? {
        let p = number(proteinG), f = number(fatG), c = number(carbG)
        guard p != nil || f != nil || c != nil else { return nil }

        return Int((((p ?? 0) * 4) + ((f ?? 0) * 9) + ((c ?? 0) * 4)).rounded())
    }

    /// 記録用の入力にする。
    ///
    /// **区分も kcal も載せない。** どちらもサーバが導出する。
    /// クライアントごとに答えがずれるのを避けるため（#191）
    func toInput(date: String, at: String) -> MealInput {
        MealInput(
            id: UUID(),
            date: date,
            at: at,
            slot: nil,
            name: text(name),
            qty: text(qty),
            kcal: nil,
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

    /// 表示している日。**前日・翌日に移動できる**（#193）
    private(set) var date: String
    /// 「今日」。これより先には進めない。テストで固定するため引数にする
    private let today: String
    private let api: APIClient

    var totals: MealTotals { MealTotals(of: meals) }
    var remaining: Macros? { target.map { totals.remaining(from: $0) } }

    init(
        api: APIClient,
        date: String = JST.dateString(),
        today: String = JST.dateString(),
        // **既定は「位置を使わない」。** 本物は PhysiqueApp が挿す
        // （`HealthSource` と同じ形）。テストが実ファイルを触らずに済む
        location: LocationSource = NoLocation(),
        places: FoodPlaceStore = FoodPlaceStore(url: nil)
    ) {
        self.api = api
        self.date = date
        self.today = today
        self.location = location
        placeStore = places
        self.places = places.load()
    }

    // MARK: - 食品マスタ（要件 N-02 / ADR-0017）

    private(set) var foodItems: [FoodItem] = []
    /// 量の入力中の項目。nil なら誰も選んでいない
    private(set) var pickingFood: FoodItem?
    /// 引数の入力量。**キーは引数の名前**
    var foodAmounts: [String: Double] = [:]
    /// 登録するときの入力。**いまの記録の入力とは別に持つ** ——
    /// 「これから食べるもの」と「登録しておきたいもの」は必ずしも同じではない
    var foodDraft = MealDraft()

    func loadFoodItems(query: String = "") async {
        let items = (try? await api.foodItems(query: query)) ?? []
        // **サーバの順（よく使う順）を、端末内の場所で並べ替える。**
        // 場所が分からなければそのまま（要件 N-08）
        foodItems = places.order(items, id: \.id, near: here)
    }

    // MARK: - よく行く場所（要件 N-08）

    /// **緯度経度はここから出ない。** サーバにも送らないし、画面にも出さない
    /// （[docs/01-要件定義.md §4-C](../../../docs/01-要件定義.md)）
    private let location: LocationSource
    private let placeStore: FoodPlaceStore
    private var places: FoodPlaces
    private var here: Coordinate?

    /// 近い順に並べているか。画面の出し分けに使う
    var nearbyOn: Bool { here != nil }

    /// まだ聞いていないか。**聞いていないときだけ誘う**
    var canOfferNearby: Bool { location.permission != .denied && here == nil }

    /// 近い順を有効にする。**ここで初めて権限を聞く。**
    ///
    /// 開いた瞬間に聞かないのは、この画面が片手で速く触るためのものだから
    /// （#188）。いきなりダイアログで止めるのは筋が悪い。
    func enableNearby() async {
        if location.permission == .notDetermined {
            _ = await location.request()
        }
        here = await location.current()
    }

    /// この場所で使ったことを覚える。**失敗しても入力は済んでいる**
    private func rememberPlace(_ item: FoodItem) {
        guard let here else { return }
        places.record(item.id, at: here)
        placeStore.save(places)
    }

    /// マスタから選ぶ。
    ///
    /// **引数が無ければその場で入力が埋まる**（1タップ）。
    /// あるときは量を聞く —— 既定値のまま確定してもよい（ADR-0017）。
    func pickFood(_ item: FoodItem) {
        // **比例する項目も量を聞く**（#218）。引数が無くても量で結果が変わる
        guard item.needsAmount else {
            apply(item.expand(), from: item)

            return
        }

        pickingFood = item
        foodAmounts = item.defaultAmounts
        // 触らなければ基準量ぶん
        foodBase = item.scales ? item.baseAmount : nil
    }

    func confirmFoodPick() {
        guard let item = pickingFood else { return }
        apply(item.expand(base: foodBase, foodAmounts), from: item)
    }

    func cancelFoodPick() {
        pickingFood = nil
        foodAmounts = [:]
        foodBase = nil
    }

    /// 登録する引数。**数値は文字列のまま持つ**（「30.」で丸められないように）
    var foodComponents: [FoodComponentDraft] = []

    // MARK: - 全量が量に比例する（#218）

    /// 「全量が量に比例する」チェック。プロテインのように引数の行を作らずに済ませる
    var foodScales = false
    /// 基準量。**文字列のまま持つ**（他の数値欄と同じ理由）
    var foodBaseAmount = ""
    var foodBaseUnit = "g"

    /// 入力中の本体の量。比例する項目を選んだときだけ使う
    var foodBase: Double?

    /// 直している項目。**nil なら新規登録**。保存先が PATCH か POST かを決める
    private(set) var editingFoodID: UUID?

    /// 登録画面を開く。**いまの入力があれば初期値に写す**（便宜）。
    /// 空でも構わないし、写したあと書き換えてもよい
    func beginRegisteringFood() {
        editingFoodID = nil
        foodDraft = draft
        // **既定は引数なし**（ADR-0017）。量が変わるものだけ足す
        foodComponents = []
        foodScales = false
        foodBaseAmount = ""
        foodBaseUnit = "g"
        errorMessage = nil
    }

    /// 登録済みの項目を直す。
    ///
    /// **ADR-0017 で引数を任意にした前提がここ。** 登録時点では量が固定だと
    /// 思っていても、2回目に「毎回違う」と気づくことがある。そのときに
    /// 引数を足せないと、消して作り直すしかなく `usedCount` が戻る。
    func beginEditingFood(_ item: FoodItem) {
        editingFoodID = item.id

        var d = MealDraft()
        d.name = item.name
        d.qty = item.qty ?? ""
        d.proteinG = item.proteinG.map(trim) ?? ""
        d.fatG = item.fatG.map(trim) ?? ""
        d.carbG = item.carbG.map(trim) ?? ""
        foodDraft = d

        foodComponents = item.components.map { FoodComponentDraft($0) }
        foodScales = item.scalesWithAmount ?? false
        foodBaseAmount = item.baseAmount.map(trim) ?? ""
        foodBaseUnit = item.baseUnit ?? "g"
        errorMessage = nil
    }

    func addFoodComponent() {
        foodComponents.append(FoodComponentDraft())
    }

    func removeFoodComponent(at index: Int) {
        guard foodComponents.indices.contains(index) else { return }
        foodComponents.remove(at: index)
    }

    /// マスタに保存する。**`editingFoodID` があれば上書き、無ければ新規。**
    ///
    /// 上書きで済ませるのは `usedCount` を残すため。消して作り直すと
    /// 並び順が先頭から落ちる。
    func saveFood(name: String) async {
        let trimmed = name.trimmingCharacters(in: .whitespaces)
        guard !trimmed.isEmpty else {
            errorMessage = "名前を入れる"

            return
        }

        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        var components: [FoodItemComponent]?
        if !foodComponents.isEmpty {
            var out: [FoodItemComponent] = []
            for d in foodComponents {
                guard let c = d.toComponent() else {
                    errorMessage = d.problem()

                    return
                }
                out.append(c)
            }
            components = out
        }

        // **比例するなら基準量が要る**（#218）。0 では割れない
        var basis: Double?
        if foodScales {
            guard let v = Double(foodBaseAmount.trimmingCharacters(in: .whitespaces)), v > 0 else {
                errorMessage = "基準量は 0 より大きい数にする"

                return
            }
            basis = v
        }

        let input = FoodItemInput(
            name: trimmed,
            qty: foodDraft.qty.isEmpty ? nil : foodDraft.qty,
            // **本体の PFC は引数があっても送る**（#218 で足し算になった）
            proteinG: Double(foodDraft.proteinG),
            fatG: Double(foodDraft.fatG),
            carbG: Double(foodDraft.carbG),
            baseAmount: basis,
            baseUnit: foodScales ? foodBaseUnit : nil,
            scalesWithAmount: foodScales,
            components: components
        )

        do {
            if let id = editingFoodID {
                _ = try await api.updateFoodItem(id: id, input)
            } else {
                _ = try await api.createFoodItem(input)
            }
            await loadFoodItems()
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription
                ?? (editingFoodID == nil ? "登録できない" : "保存できない")
        }
    }

    /// マスタから消す。**サーバ側は論理削除**（ADR-0014）。
    func deleteFood(_ item: FoodItem) async {
        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        do {
            try await api.deleteFoodItem(id: item.id)
            await loadFoodItems()
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "削除できない"
        }
    }

    /// 展開した結果を入力欄に入れ、使った回数を増やす。
    private func apply(_ macros: Macros, from item: FoodItem) {
        draft.proteinG = trim(macros.proteinG)
        draft.fatG = trim(macros.fatG)
        draft.carbG = trim(macros.carbG)
        if draft.name.isEmpty { draft.name = item.name }
        // **登録時ではなく選んだときに覚える。** 登録は家でまとめてやることが
        // あるが、選ぶのは食べる場所（要件 N-08）
        rememberPlace(item)
        cancelFoodPick()

        // **次回から上位に出す。** 失敗しても入力は済んでいるので握る
        Task { try? await api.markFoodItemUsed(id: item.id) }
    }

    // MARK: - 目標を手で決める（#195）

    /// 手動目標の入力。**PFC だけ**（kcal は計算される）
    var manualDraft = MealDraft()
    /// いまの目標が手で決めた値か。画面でそう見せるため
    private(set) var targetIsManual = false

    func loadManualTarget() async {
        guard let t = try? await api.manualTargets() else { return }

        var d = MealDraft()
        d.proteinG = trim(t.proteinG)
        d.fatG = trim(t.fatG)
        d.carbG = trim(t.carbG)
        manualDraft = d
    }

    func saveManualTarget() async {
        // **PFC は3つで1組。** 1つ欠けた目標は意味を成さない
        guard let p = Double(manualDraft.proteinG.trimmingCharacters(in: .whitespaces)),
              let f = Double(manualDraft.fatG.trimmingCharacters(in: .whitespaces)),
              let c = Double(manualDraft.carbG.trimmingCharacters(in: .whitespaces)) else {
            errorMessage = "P・F・C を3つとも入れる"

            return
        }

        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        do {
            _ = try await api.putManualTargets(
                ManualTargets(proteinG: p, fatG: f, carbG: c, kcal: nil))
            await reloadTargets()
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "目標を保存できない"
        }
    }

    /// 手動目標を消す。自動計算（A-02）に戻る
    func clearManualTarget() async {
        isWorking = true
        defer { isWorking = false }

        do {
            try await api.deleteManualTargets()
            manualDraft = MealDraft()
            await reloadTargets()
        } catch {
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "目標を消せない"
        }
    }

    // MARK: - 記録を直す（#193）

    /// 開いている行。nil なら誰も開いていない
    private(set) var editingID: UUID?
    var editDraft = MealDraft()
    /// 編集中の時刻 "HH:MM"。**空なら時刻を消す**
    var editTime = ""

    /// 行を開く。**いまの値を入れておく** —— 直したいのは一部なので、
    /// 空から打ち直させない
    func beginEditing(_ meal: Meal) {
        editingID = meal.id
        editTime = meal.at ?? ""
        errorMessage = nil

        var d = MealDraft()
        d.name = meal.name ?? ""
        d.qty = meal.qty ?? ""
        // **0 を空欄にしない。** 「0 と記録した」と「入れていない」は別
        d.proteinG = meal.proteinG.map { trim($0) } ?? ""
        d.fatG = meal.fatG.map { trim($0) } ?? ""
        d.carbG = meal.carbG.map { trim($0) } ?? ""
        editDraft = d
    }

    func cancelEditing() {
        editingID = nil
        editDraft = MealDraft()
        editTime = ""
    }

    func saveEdit() async {
        guard let id = editingID else { return }
        errorMessage = nil
        isWorking = true
        defer { isWorking = false }

        var input = editDraft.toInput(date: date, at: editTime)
        // 新規記録と違い、id はサーバ側のものを使う（採番し直さない）
        input.id = nil

        do {
            let updated = try await api.updateMeal(id: id, input)
            if let i = meals.firstIndex(where: { $0.id == id }) {
                meals[i] = updated
            }
            // 時刻を直すと並び順が変わる
            meals.sort { ($0.at ?? "~") < ($1.at ?? "~") }
            cancelEditing()
        } catch {
            // **閉じない。** 閉じると打ち直しになる
            errorMessage = (error as? LocalizedError)?.errorDescription ?? "直せない"
        }
    }

    // MARK: - 日付の行き来（#193）

    /// `9/19(土)` の形。
    var dateLabel: String { JST.displayString(from: date) }

    /// **今日より先には進めない。** 記録できない日を開いても意味が無い
    var canGoNext: Bool { date < today }

    func goToPreviousDay() {
        move(to: JST.shift(date, days: -1))
    }

    func goToNextDay() {
        guard canGoNext else { return }
        move(to: JST.shift(date, days: 1))
    }

    /// 日付を選び直す。未来を選んだら今日に丸める
    func goTo(_ newDate: String) {
        move(to: min(newDate, today))
    }

    private func move(to newDate: String) {
        guard newDate != date else { return }
        date = newDate
        // 前の日の記録が残ったまま見えないよう、先に空にする
        meals = []
        target = nil
        targetsMessage = nil
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
        await reloadTargets()
    }

    /// 目標を取り直す。**目標は取れなくてもよい** —— フェーズも手動目標も
    /// 無いと 422 になるが、ここで止めると記録そのものができなくなる
    private func reloadTargets() async {
        do {
            let t = try await api.dailyTargets(date: date)
            target = t.target
            targetIsManual = t.targetSource == "manual"
            targetsMessage = t.target == nil ? t.note : nil
        } catch let e as APIError {
            target = nil
            targetIsManual = false
            targetsMessage = e.errorDescription
        } catch {
            target = nil
            targetIsManual = false
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
        // kcal は入れない。PFC から計算される
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
            // **時刻はここで決める。** 記録ボタンを押した時刻が「食べた時刻」
            let created = try await api.createMeal(
                draft.toInput(date: date, at: JST.timeString()))
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
    private func trim(_ v: Double) -> String { numberText(v) }
}
