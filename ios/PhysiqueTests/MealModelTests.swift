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
