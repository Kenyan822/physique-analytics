import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

private func api(_ responses: [(String, Int)]) -> (APIClient, FakeTransport) {
    let t = FakeTransport()
    t.responses = responses.map { (Data($0.0.utf8), $0.1) }

    return (APIClient(baseURL: base, transport: t), t)
}

private let emptyMeals = (#"{"items":[]}"#, 200)
private let targets = ("""
{"date":"2026-09-16","target":{"kcal":2400,"proteinG":180,"fatG":70,"carbG":250},
 "consumed":{"kcal":0,"proteinG":0,"fatG":0,"carbG":0}}
""", 200)

@Suite("食事の記録")
@MainActor
struct MealModelTests {
    @Test("読み込むと合計と残量が出る")
    func load() async {
        let (client, _) = api([
            (#"{"items":[{"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-16","name":"鶏むね","kcal":330,"proteinG":62,"fatG":7,"carbG":0}]}"#, 200),
            targets,
        ])
        let model = MealModel(api: client, date: "2026-09-16")

        await model.load()

        #expect(model.totals.kcal == 330)
        #expect(model.remaining?.kcal == 2070)
    }

    @Test("**目標が出せなくても記録は続けられる**")
    func targetsUnavailable() async {
        // フェーズ未登録だと 422。ここで落とすと記録そのものができなくなる
        let (client, _) = api([
            emptyMeals,
            (#"{"type":"about:blank","status":422,"title":"入力が仕様に合わない","detail":"フェーズが1つも登録されていない"}"#, 422),
        ])
        let model = MealModel(api: client, date: "2026-09-16")

        await model.load()

        #expect(model.remaining == nil)
        #expect(model.targetsMessage?.contains("フェーズ") == true)
        // 記録の一覧は読めている
        #expect(model.meals.isEmpty)
        #expect(model.errorMessage == nil)
    }

    @Test("記録すると一覧に増える")
    func record() async {
        let (client, _) = api([
            emptyMeals,
            targets,
            (#"{"id":"22222222-2222-2222-2222-222222222222","date":"2026-09-16","name":"卵","kcal":80,"source":"manual"}"#, 201),
        ])
        let model = MealModel(api: client, date: "2026-09-16")
        await model.load()

        model.draft.name = "卵"
        model.draft.proteinG = "7"
        model.draft.fatG = "5"
        model.draft.carbG = "0.3"
        await model.record()

        #expect(model.meals.count == 1)
        #expect(model.totals.kcal == 80)
        // 続けて入れられるよう入力欄は空に戻す
        #expect(model.draft.name.isEmpty)
    }

    @Test("名前が空なら記録しない")
    func requiresName() async {
        let (client, t) = api([emptyMeals, targets])
        let model = MealModel(api: client, date: "2026-09-16")
        await model.load()

        model.draft.name = "   "
        await model.record()

        // 送信していないこと
        #expect(t.requests.count == 2)
    }

    @Test("候補を選ぶと入力が埋まる")
    func pickSuggestion() async {
        let (client, _) = api([emptyMeals, targets])
        let model = MealModel(api: client, date: "2026-09-16")
        await model.load()

        model.pick(MealSuggestion(
            name: "鶏むね", count: 12, lastDate: nil, qty: "200g",
            kcal: 330, proteinG: 62, fatG: 7, carbG: 0
        ))

        // **選んだ時点で入力が終わる**のが狙い
        #expect(model.draft.name == "鶏むね")
        #expect(model.draft.proteinG == "62")
        // kcal は候補から写さず、PFC から出す
        #expect(model.draft.kcal == 62 * 4 + 7 * 9 + 0 * 4)
    }

    // 区分の初期選択は消えた。**サーバが時刻から導出する**ので
    // クライアントは持たない（#191）。送る内容の検証は MealTests の
    // 「記録に時刻を載せる」にある
}

@Suite("入力の読み取り")
struct MealDraftTests {
    @Test("数値に直す")
    func parses() {
        var d = MealDraft()
        d.name = " 鶏むね "
        d.proteinG = "62.5"

        let input = d.toInput(date: "2026-09-16", at: "12:00")

        #expect(input.name == "鶏むね")
        // kcal は送らない。サーバが PFC から計算する（#188）
        #expect(input.kcal == nil)
        #expect(input.proteinG == 62.5)
    }

    @Test("**空欄は 0 ではなく未入力**")
    func emptyIsNil() {
        // 0 として送ると「食べたが記録が雑」と「0 kcal」が同じになる
        var d = MealDraft()
        d.name = "水"

        let input = d.toInput(date: "2026-09-16", at: "22:30")

        #expect(input.kcal == nil)
        #expect(input.proteinG == nil)
    }

    @Test("読めない値は未入力にする")
    func garbage() {
        var d = MealDraft()
        d.name = "x"
        d.proteinG = "だいたい30"

        #expect(d.toInput(date: "2026-09-16", at: "12:00").proteinG == nil)
        #expect(d.kcal == nil)
    }
}

@Suite("日付を行き来する")
@MainActor
struct MealDateTests {
    private func model(date: String) -> MealModel {
        let (client, _) = api([emptyMeals, targets])

        return MealModel(api: client, date: date, today: "2026-09-19")
    }

    @Test("前日に戻れる")
    func previous() {
        let m = model(date: "2026-09-19")
        m.goToPreviousDay()

        #expect(m.date == "2026-09-18")
    }

    @Test("翌日に進める")
    func next() {
        let m = model(date: "2026-09-17")
        m.goToNextDay()

        #expect(m.date == "2026-09-18")
    }

    @Test("月をまたぐ")
    func acrossMonths() {
        let m = model(date: "2026-10-01")
        m.goToPreviousDay()

        #expect(m.date == "2026-09-30")
    }

    // **未来には進めない。** 記録できない日を開いても意味が無い
    @Test("**今日より先には進めない**")
    func noFuture() {
        let m = model(date: "2026-09-19")
        #expect(!m.canGoNext)

        m.goToNextDay()
        #expect(m.date == "2026-09-19")
    }

    @Test("今日より前なら進める")
    func canAdvanceFromPast() {
        #expect(model(date: "2026-09-18").canGoNext)
    }

    @Test("表示は 9/19(土) の形")
    func label() {
        #expect(model(date: "2026-09-19").dateLabel == JST.displayString(from: "2026-09-19"))
    }
}

@Suite("記録を直す")
@MainActor
struct MealEditTests {
    private let existing = #"""
    {"items":[{"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-19",
      "at":"19:40","slot":"夕食","name":"鶏むね","qty":"200g",
      "kcal":220,"proteinG":45,"fatG":5,"carbG":0,"source":"manual"}]}
    """#

    private func loaded() async -> (MealModel, FakeTransport) {
        let (client, t) = api([(existing, 200), targets])
        let m = MealModel(api: client, date: "2026-09-19")
        await m.load()

        return (m, t)
    }

    @Test("開くと今の値が入る")
    func beginEdit() async {
        let (m, _) = await loaded()
        let meal = m.meals[0]

        m.beginEditing(meal)

        #expect(m.editingID == meal.id)
        #expect(m.editDraft.proteinG == "45")
        #expect(m.editDraft.name == "鶏むね")
        #expect(m.editDraft.qty == "200g")
        #expect(m.editTime == "19:40")
    }

    @Test("**0 は 0 として出す。** 空欄と混ぜない")
    func keepsZero() async {
        let (m, _) = await loaded()
        m.beginEditing(m.meals[0])

        #expect(m.editDraft.carbG == "0")
    }

    @Test("閉じると編集をやめる")
    func cancel() async {
        let (m, _) = await loaded()
        m.beginEditing(m.meals[0])
        m.cancelEditing()

        #expect(m.editingID == nil)
    }

    @Test("保存すると一覧が置き換わる")
    func save() async {
        let (client, _) = api([
            (existing, 200),
            targets,
            (#"""
            {"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-19",
             "at":"20:10","slot":"夕食","name":"鶏むね","kcal":260,
             "proteinG":50,"fatG":6,"carbG":0,"source":"manual",
             "createdAt":"2026-09-19T10:00:00Z","updatedAt":"2026-09-19T11:00:00Z"}
            """#, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-19")
        await m.load()

        m.beginEditing(m.meals[0])
        m.editDraft.proteinG = "50"
        m.editTime = "20:10"
        await m.saveEdit()

        #expect(m.editingID == nil)
        #expect(m.meals.count == 1)
        #expect(m.meals[0].proteinG == 50)
        // 時刻を直すと区分も付け直される（サーバが導出する）
        #expect(m.meals[0].at == "20:10")
    }

    @Test("失敗したら開いたままにする")
    func keepsOpenOnFailure() async {
        let (client, _) = api([
            (existing, 200),
            targets,
            (#"{"type":"about:blank","title":"だめ","status":422}"#, 422),
        ])
        let m = MealModel(api: client, date: "2026-09-19")
        await m.load()

        m.beginEditing(m.meals[0])
        m.editDraft.proteinG = "50"
        await m.saveEdit()

        // **閉じない。** 閉じると打ち直しになる
        #expect(m.editingID != nil)
        #expect(m.errorMessage != nil)
    }
}

@Suite("目標を手で決める")
@MainActor
struct ManualTargetTests {
    private let noManual = (#"{"targets":null}"#, 200)
    private let manual = (#"{"targets":{"proteinG":180,"fatG":70,"carbG":250,"kcal":2350}}"#, 200)

    @Test("手動目標が入っていれば読める")
    func loadsManual() async {
        let (client, _) = api([emptyMeals, targets, manual])
        let m = MealModel(api: client, date: "2026-09-16")
        await m.load()
        await m.loadManualTarget()

        #expect(m.manualDraft.proteinG == "180")
        #expect(m.manualDraft.carbG == "250")
    }

    @Test("未設定なら空のまま")
    func emptyWhenUnset() async {
        let (client, _) = api([emptyMeals, targets, noManual])
        let m = MealModel(api: client, date: "2026-09-16")
        await m.load()
        await m.loadManualTarget()

        #expect(m.manualDraft.proteinG == "")
    }

    @Test("保存すると目標が更新される")
    func saves() async {
        let (client, _) = api([
            emptyMeals, targets, noManual,
            (#"{"proteinG":200,"fatG":60,"carbG":200,"kcal":2180}"#, 200),
            (#"""
            {"date":"2026-09-16","targetSource":"manual",
             "target":{"kcal":2180,"proteinG":200,"fatG":60,"carbG":200},
             "consumed":{"kcal":0,"proteinG":0,"fatG":0,"carbG":0},
             "remaining":{"kcal":2180,"proteinG":200,"fatG":60,"carbG":200}}
            """#, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-16")
        await m.load()
        await m.loadManualTarget()

        m.manualDraft.proteinG = "200"
        m.manualDraft.fatG = "60"
        m.manualDraft.carbG = "200"
        await m.saveManualTarget()

        #expect(m.target?.proteinG == 200)
        #expect(m.targetIsManual)
    }

    @Test("**空欄があれば保存しない**")
    func requiresAllThree() async {
        let (client, t) = api([emptyMeals, targets, noManual])
        let m = MealModel(api: client, date: "2026-09-16")
        await m.load()
        await m.loadManualTarget()

        m.manualDraft.proteinG = "200"
        let before = t.requests.count
        await m.saveManualTarget()

        // PFC は3つで1組。1つ欠けた目標は意味を成さない
        #expect(t.requests.count == before)
        #expect(m.errorMessage != nil)
    }
}

@Suite("食品マスタから選ぶ")
@MainActor
struct FoodMasterPickTests {
    private let egg = #"""
    {"items":[{"id":"11111111-1111-1111-1111-111111111111","name":"ゆで卵","qty":"1個",
      "proteinG":6.5,"fatG":5.2,"carbG":0.2,"components":[],"usedCount":3,
      "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
    """#

    private let protein = #"""
    {"items":[{"id":"22222222-2222-2222-2222-222222222222","name":"プロテイン",
      "components":[{"name":"量","unit":"g","basisAmount":30,"defaultAmount":30,
        "proteinG":24,"fatG":1.5,"carbG":2}],"usedCount":0,
      "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
    """#

    private func loaded(_ foods: String) async -> (MealModel, FakeTransport) {
        let (client, t) = api([emptyMeals, targets, (foods, 200)])
        let m = MealModel(api: client, date: "2026-09-21")
        await m.load()
        await m.loadFoodItems()

        return (m, t)
    }

    @Test("一覧を読める")
    func loads() async {
        let (m, _) = await loaded(egg)

        #expect(m.foodItems.count == 1)
        #expect(m.foodItems[0].name == "ゆで卵")
    }

    @Test("**引数が無ければ選んだ時点で入力が埋まる**")
    func picksSimple() async {
        let (m, _) = await loaded(egg)

        m.pickFood(m.foodItems[0])

        #expect(m.draft.proteinG == "6.5")
        #expect(m.draft.carbG == "0.2")
        // 引数が無いので量の入力は出さない
        #expect(m.pickingFood == nil)
    }

    @Test("**引数があれば量の入力に入る**")
    func picksWithComponents() async {
        let (m, _) = await loaded(protein)

        m.pickFood(m.foodItems[0])

        #expect(m.pickingFood?.name == "プロテイン")
        // 既定値が入っている
        #expect(m.foodAmounts["量"] == 30)
    }

    @Test("量を変えると PFC が比例する")
    func scales() async {
        let (m, _) = await loaded(protein)
        m.pickFood(m.foodItems[0])

        m.foodAmounts["量"] = 45
        m.confirmFoodPick()

        #expect(m.draft.proteinG == "36")
        #expect(m.pickingFood == nil)
    }

    @Test("触らなければ既定値で入る")
    func usesDefault() async {
        let (m, _) = await loaded(protein)
        m.pickFood(m.foodItems[0])

        m.confirmFoodPick()

        #expect(m.draft.proteinG == "24")
    }

    @Test("やめると入力が変わらない")
    func cancels() async {
        let (m, _) = await loaded(protein)
        m.draft.proteinG = "1"
        m.pickFood(m.foodItems[0])

        m.cancelFoodPick()

        #expect(m.pickingFood == nil)
        #expect(m.draft.proteinG == "1")
    }

    @Test("マスタに登録できる")
    func registers() async {
        let (client, t) = api([
            emptyMeals, targets, (#"{"items":[]}"#, 200),
            (#"""
            {"id":"33333333-3333-3333-3333-333333333333","name":"新しい","components":[],
             "usedCount":0,"createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}
            """#, 201),
            (#"{"items":[]}"#, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-21")
        await m.load()
        await m.loadFoodItems()

        // **いまの入力とは切り離す。** 登録用の入力欄を別に持つ
        m.foodDraft.proteinG = "24"
        m.foodDraft.fatG = "1.5"
        await m.saveFood(name: "新しい")

        // POST が飛んでいる
        #expect(t.requests.contains { $0.httpMethod == "POST" && $0.url?.path == "/v1/food-items" })
    }

    @Test("**入力が空でも登録画面は開ける**")
    func opensWithEmptyDraft() async {
        let (m, _) = await loaded(egg)

        // いまの入力を写すが、空でも構わない
        m.beginRegisteringFood()

        #expect(m.foodDraft.isEmpty)
    }

    @Test("いまの入力があれば初期値に写す")
    func copiesDraft() async {
        let (m, _) = await loaded(egg)
        m.draft.proteinG = "24"

        m.beginRegisteringFood()

        #expect(m.foodDraft.proteinG == "24")
    }

    @Test("**名前が空なら登録しない**")
    func requiresName() async {
        let (m, t) = await loaded(egg)
        let before = t.requests.count

        await m.saveFood(name: "   ")

        #expect(t.requests.count == before)
        #expect(m.errorMessage != nil)
    }
}

@Suite("引数つきで登録する")
@MainActor
struct FoodComponentEditTests {
    private func model() async -> (MealModel, FakeTransport) {
        let (client, t) = api([
            emptyMeals, targets, (#"{"items":[]}"#, 200),
            (#"""
            {"id":"33333333-3333-3333-3333-333333333333","name":"プロテイン",
             "components":[{"name":"量","unit":"g","basisAmount":30,"defaultAmount":30,
               "proteinG":24,"fatG":1.5,"carbG":2}],"usedCount":0,
             "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}
            """#, 201),
            (#"{"items":[]}"#, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-21")
        await m.load()
        await m.loadFoodItems()
        m.beginRegisteringFood()

        return (m, t)
    }

    @Test("**既定は引数なし**")
    func startsWithout() async {
        let (m, _) = await model()

        #expect(m.foodComponents.isEmpty)
    }

    @Test("引数を足せる")
    func adds() async {
        let (m, _) = await model()

        m.addFoodComponent()

        #expect(m.foodComponents.count == 1)
        // **基準量の既定は 100。** パッケージの表示が「100g あたり」が多い
        #expect(m.foodComponents[0].basisAmount == "100")
    }

    @Test("引数を消せる")
    func removes() async {
        let (m, _) = await model()
        m.addFoodComponent()
        m.addFoodComponent()

        m.removeFoodComponent(at: 0)

        #expect(m.foodComponents.count == 1)
    }

    @Test("引数つきで送る")
    func sends() async {
        let (m, t) = await model()
        m.addFoodComponent()
        m.foodComponents[0].name = "量"
        m.foodComponents[0].basisAmount = "30"
        m.foodComponents[0].defaultAmount = "30"
        m.foodComponents[0].proteinG = "24"

        await m.saveFood(name: "プロテイン")

        let req = try! #require(t.requests.first { $0.httpMethod == "POST" })
        let body = try! JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        let cs = body["components"] as! [[String: Any]]

        #expect(cs.count == 1)
        #expect(cs[0]["basisAmount"] as? Double == 30)
    }

    @Test("**基準量が空なら登録しない**")
    func requiresBasis() async {
        let (m, t) = await model()
        m.addFoodComponent()
        m.foodComponents[0].name = "量"
        m.foodComponents[0].basisAmount = ""
        let before = t.requests.count

        await m.saveFood(name: "x")

        #expect(t.requests.count == before)
        #expect(m.errorMessage != nil)
    }

    @Test("引数の名前が空なら登録しない")
    func requiresComponentName() async {
        let (m, t) = await model()
        m.addFoodComponent()
        m.foodComponents[0].name = "  "
        let before = t.requests.count

        await m.saveFood(name: "x")

        #expect(t.requests.count == before)
    }
}

@Suite("登録した食品を直す")
@MainActor
struct FoodItemEditTests {
    /// 引数つき。**あとから量を変えたくなった**ケース
    private let protein = #"""
    {"items":[{"id":"44444444-4444-4444-4444-444444444444","name":"プロテイン","qty":"1杯",
      "components":[{"name":"量","unit":"g","basisAmount":30,"defaultAmount":30,
        "proteinG":24,"fatG":1.5,"carbG":2}],"usedCount":5,
      "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
    """#

    /// 引数なし。**ここに引数を足せることが ADR-0017 の前提**
    private let egg = #"""
    {"items":[{"id":"55555555-5555-5555-5555-555555555555","name":"ゆで卵","qty":"1個",
      "proteinG":6.5,"fatG":5.2,"carbG":0.2,"components":[],"usedCount":3,
      "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
    """#

    private func loaded(_ foods: String) async -> (MealModel, FakeTransport) {
        let (client, t) = api([
            emptyMeals, targets, (foods, 200),
            // 保存・削除の応答。最後の1つは使い回されるので一覧を置く
            (#"""
            {"id":"44444444-4444-4444-4444-444444444444","name":"x","components":[],
             "usedCount":5,"createdAt":"2026-09-21T00:00:00Z",
             "updatedAt":"2026-09-21T00:00:00Z"}
            """#, 200),
            (foods, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-21")
        await m.load()
        await m.loadFoodItems()

        return (m, t)
    }

    @Test("開くと今の値が入る")
    func fillsDraft() async {
        let (m, _) = await loaded(egg)

        m.beginEditingFood(m.foodItems[0])

        #expect(m.foodDraft.name == "ゆで卵")
        #expect(m.foodDraft.qty == "1個")
        #expect(m.foodDraft.proteinG == "6.5")
    }

    @Test("引数つきを開くと引数も入る")
    func fillsComponents() async {
        let (m, _) = await loaded(protein)

        m.beginEditingFood(m.foodItems[0])

        #expect(m.foodComponents.count == 1)
        #expect(m.foodComponents[0].name == "量")
        // **文字列に戻す。** 「30.0」だと打ち直しづらい
        #expect(m.foodComponents[0].basisAmount == "30")
        #expect(m.foodComponents[0].proteinG == "24")
    }

    @Test("**引数なしの項目にあとから引数を足せる**（ADR-0017 の前提）")
    func addsComponentLater() async throws {
        let (m, t) = await loaded(egg)
        m.beginEditingFood(m.foodItems[0])

        m.addFoodComponent()
        m.foodComponents[0].name = "個数"
        m.foodComponents[0].basisAmount = "1"
        m.foodComponents[0].proteinG = "6.5"
        await m.saveFood(name: "ゆで卵")

        let req = try #require(t.requests.last { $0.httpMethod == "PATCH" })
        let body = try JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        let cs = try #require(body["components"] as? [[String: Any]])
        #expect(cs.count == 1)
        #expect(cs[0]["name"] as? String == "個数")
    }

    @Test("**PATCH で送る。** POST だと別の項目が増える")
    func usesPatch() async throws {
        let (m, t) = await loaded(egg)
        m.beginEditingFood(m.foodItems[0])

        await m.saveFood(name: "ゆでたまご")

        let req = try #require(t.requests.last { $0.httpMethod == "PATCH" })
        #expect(req.url?.path.hasSuffix("/v1/food-items/55555555-5555-5555-5555-555555555555") == true)
        #expect(!t.requests.contains { $0.httpMethod == "POST" })
    }

    @Test("登録に戻ると POST になる")
    func registerAfterEdit() async throws {
        let (m, t) = await loaded(egg)
        m.beginEditingFood(m.foodItems[0])

        m.beginRegisteringFood()
        await m.saveFood(name: "新しいもの")

        #expect(t.requests.contains { $0.httpMethod == "POST" })
        #expect(!t.requests.contains { $0.httpMethod == "PATCH" })
        // 引数も持ち越さない
        #expect(m.foodComponents.isEmpty)
    }

    @Test("消せる")
    func deletes() async throws {
        let (m, t) = await loaded(egg)

        await m.deleteFood(m.foodItems[0])

        let req = try #require(t.requests.last { $0.httpMethod == "DELETE" })
        #expect(req.url?.path.hasSuffix("/v1/food-items/55555555-5555-5555-5555-555555555555") == true)
    }

    @Test("名前が空なら保存しない")
    func requiresName() async {
        let (m, t) = await loaded(egg)
        m.beginEditingFood(m.foodItems[0])
        let before = t.requests.count

        await m.saveFood(name: "  ")

        #expect(t.requests.count == before)
        #expect(m.errorMessage != nil)
    }
}

@Suite("全量が量に比例する（#218）")
@MainActor
struct FoodScalingTests {
    private let protein = #"""
    {"items":[{"id":"66666666-6666-6666-6666-666666666666","name":"プロテイン",
      "proteinG":24,"fatG":1.5,"carbG":2,"baseAmount":30,"baseUnit":"g",
      "scalesWithAmount":true,"components":[],"usedCount":2,
      "createdAt":"2026-09-21T00:00:00Z","updatedAt":"2026-09-21T00:00:00Z"}]}
    """#

    private func loaded(_ foods: String) async -> (MealModel, FakeTransport) {
        let (client, t) = api([
            emptyMeals, targets, (foods, 200),
            (#"""
            {"id":"66666666-6666-6666-6666-666666666666","name":"x","components":[],
             "usedCount":0,"createdAt":"2026-09-21T00:00:00Z",
             "updatedAt":"2026-09-21T00:00:00Z"}
            """#, 201),
            (foods, 200),
        ])
        let m = MealModel(api: client, date: "2026-09-21")
        await m.load()
        await m.loadFoodItems()

        return (m, t)
    }

    @Test("**比例する項目は選ぶと量を聞く**")
    func asksAmount() async {
        let (m, _) = await loaded(protein)

        m.pickFood(m.foodItems[0])

        // 引数が無くても量を聞く（#218）
        #expect(m.pickingFood != nil)
    }

    @Test("量を入れると本体が比例する")
    func scales() async {
        let (m, _) = await loaded(protein)
        m.pickFood(m.foodItems[0])

        m.foodBase = 45
        m.confirmFoodPick()

        #expect(m.draft.proteinG == "36")
        #expect(m.draft.fatG == "2.25")
    }

    @Test("触らなければ基準量ぶん")
    func defaultsToBasis() async {
        let (m, _) = await loaded(protein)
        m.pickFood(m.foodItems[0])

        m.confirmFoodPick()

        #expect(m.draft.proteinG == "24")
    }

    @Test("編集で開くと比例の設定が入る")
    func fillsScaling() async {
        let (m, _) = await loaded(protein)

        m.beginEditingFood(m.foodItems[0])

        #expect(m.foodScales)
        #expect(m.foodBaseAmount == "30")
        #expect(m.foodBaseUnit == "g")
    }

    @Test("**比例を付けて登録できる**")
    func registersScaling() async throws {
        let (m, t) = await loaded(#"{"items":[]}"#)
        m.beginRegisteringFood()
        m.foodDraft.proteinG = "24"
        m.foodScales = true
        m.foodBaseAmount = "30"

        await m.saveFood(name: "プロテイン")

        let req = try #require(t.requests.last { $0.httpMethod == "POST" })
        let body = try JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        #expect(body["scalesWithAmount"] as? Bool == true)
        #expect(body["baseAmount"] as? Double == 30)
        // **本体の PFC を落とさない**（#218 で足し算になった）
        #expect(body["proteinG"] as? Double == 24)
    }

    @Test("**基準量が空なら登録しない**")
    func requiresBasis() async {
        let (m, t) = await loaded(#"{"items":[]}"#)
        m.beginRegisteringFood()
        m.foodScales = true
        m.foodBaseAmount = ""
        let before = t.requests.count

        await m.saveFood(name: "x")

        #expect(t.requests.count == before)
        #expect(m.errorMessage != nil)
    }

    @Test("比例を切ると基準量を送らない")
    func offSendsNothing() async throws {
        let (m, t) = await loaded(#"{"items":[]}"#)
        m.beginRegisteringFood()
        m.foodDraft.proteinG = "24"
        m.foodScales = false
        m.foodBaseAmount = "30"

        await m.saveFood(name: "ゆで卵")

        let req = try #require(t.requests.last { $0.httpMethod == "POST" })
        let body = try JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        #expect(body["scalesWithAmount"] as? Bool == false)
        #expect(body["baseAmount"] == nil)
    }
}
