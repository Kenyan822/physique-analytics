import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

private let created = #"""
{"id":"11111111-1111-1111-1111-111111111111","date":"2026-10-08","at":"20:30",
 "kcal":1200,"proteinG":60,"fatG":40,"carbG":150,"source":"rough"}
"""#

@Suite("ざっくり入力")
@MainActor
struct RoughMealTests {
    private func loaded() async -> (MealModel, FakeTransport) {
        let t = FakeTransport()
        t.responses = [
            (Data(#"{"items":[]}"#.utf8), 200),   // listMeals
            (Data(#"{"items":[]}"#.utf8), 200),   // meal-sets など
        ]
        let m = MealModel(api: APIClient(baseURL: base, transport: t), date: "2026-10-08")
        await m.load()

        return (m, t)
    }

    @Test("**kcal だけで記録できる**")
    func recordsKcalOnly() async {
        let (m, t) = await loaded()
        t.responses = [(Data(created.utf8), 201)]

        m.roughKcal = "1200"
        await m.recordRough()

        let req = try! #require(t.requests.last)
        #expect(req.httpMethod == "POST")
        #expect(req.url?.path == "/v1/meals")

        let body = try! JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        #expect(body["kcal"] as? Int == 1200)
        #expect(body["source"] as? String == "rough")
        // **PFC は送らない。** サーバが按分する
        #expect(body["proteinG"] == nil)
        #expect(body["fatG"] == nil)
        #expect(body["carbG"] == nil)
    }

    @Test("按分された結果が一覧に出る")
    func appearsInList() async {
        let (m, t) = await loaded()
        t.responses = [(Data(created.utf8), 201)]

        m.roughKcal = "1200"
        await m.recordRough()

        #expect(m.meals.count == 1)
        #expect(m.meals.first?.source == .rough)
        #expect(m.meals.first?.proteinG == 60)
    }

    @Test("記録したら入力を空に戻す")
    func clears() async {
        let (m, t) = await loaded()
        t.responses = [(Data(created.utf8), 201)]

        m.roughKcal = "1200"
        await m.recordRough()

        #expect(m.roughKcal.isEmpty)
    }

    @Test("**数字でなければ送らない**")
    func rejectsNonNumber() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        m.roughKcal = "だいたい1200"
        await m.recordRough()

        #expect(t.requests.count == n)
    }

    @Test("空欄なら送らない")
    func rejectsEmpty() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        m.roughKcal = ""
        await m.recordRough()

        #expect(t.requests.count == n)
    }

    @Test("0 以下なら送らない")
    func rejectsNonPositive() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        m.roughKcal = "0"
        await m.recordRough()

        #expect(t.requests.count == n)
    }
}
