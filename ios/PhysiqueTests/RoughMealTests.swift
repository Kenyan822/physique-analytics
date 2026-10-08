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
            (Data(#"{"items":[]}"#.utf8), 200),
            (Data(#"{"items":[]}"#.utf8), 200),
        ]
        let m = MealModel(api: APIClient(baseURL: base, transport: t), date: "2026-10-08")
        await m.load()

        return (m, t)
    }

    @Test("**杯数と食事の量から kcal を出す**")
    func estimatesFromCounts() async {
        let (m, _) = await loaded()

        m.roughDrinks = 3
        m.roughMealSize = .normal

        // 酒 150 × 3 + 普通 600
        #expect(m.roughKcal == "1050")
    }

    @Test("食事を摂っていなければ酒だけ")
    func drinksOnly() async {
        let (m, _) = await loaded()

        m.roughDrinks = 2
        m.roughMealSize = .none

        #expect(m.roughKcal == "300")
    }

    @Test("**計算結果を手で直せる**")
    func manualOverride() async {
        let (m, _) = await loaded()
        m.roughDrinks = 3
        m.roughMealSize = .normal

        m.roughKcal = "800"

        // 直したあとに杯数を触らなければ、直した値が残る
        #expect(m.roughKcal == "800")
    }

    @Test("直したあとに杯数を変えたら計算し直す")
    func recalculates() async {
        let (m, _) = await loaded()
        m.roughDrinks = 3
        m.roughMealSize = .normal
        m.roughKcal = "800"

        m.roughDrinks = 4

        #expect(m.roughKcal == "1200")
    }

    @Test("杯数は0未満にならない")
    func noNegative() async {
        let (m, _) = await loaded()

        m.roughDrinks = 0
        m.bumpDrinks(-1)

        #expect(m.roughDrinks == 0)
    }

    @Test("kcal だけで記録できる")
    func recordsKcalOnly() async {
        let (m, t) = await loaded()
        t.responses = [(Data(created.utf8), 201)]

        m.roughDrinks = 3
        m.roughMealSize = .normal
        await m.recordRough()

        let req = try! #require(t.requests.last)
        let body = try! JSONSerialization.jsonObject(with: req.httpBody!) as! [String: Any]
        #expect(body["kcal"] as? Int == 1050)
        #expect(body["source"] as? String == "rough")
        // **PFC は送らない。** サーバが按分する
        #expect(body["proteinG"] == nil)
    }

    @Test("記録したら入力を空に戻す")
    func clears() async {
        let (m, t) = await loaded()
        t.responses = [(Data(created.utf8), 201)]
        m.roughDrinks = 3

        await m.recordRough()

        #expect(m.roughDrinks == 0)
        #expect(m.roughMealSize == .none)
    }

    @Test("**0 kcal では送らない**")
    func rejectsZero() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        await m.recordRough()

        #expect(t.requests.count == n)
    }
}
