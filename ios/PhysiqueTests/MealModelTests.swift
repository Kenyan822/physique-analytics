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
