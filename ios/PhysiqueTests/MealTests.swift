import Foundation
import Testing

@testable import PhysiqueCore

@Suite("食事の区分")
struct MealSlotTests {
    /// **JST で組み立てる。** 素の `Calendar` は端末のタイムゾーンで Date を作るので、
    /// UTC で走る CI だと9時間ずれて全ケースが落ちる。
    private func slot(hour: Int) -> MealSlot {
        var c = DateComponents()
        c.year = 2026; c.month = 9; c.day = 16; c.hour = hour
        let date = JST.calendar.date(from: c)!

        return MealSlot.suggested(at: date)
    }

    @Test("時刻から区分を推測する")
    func suggests() {
        // 開くたびに選び直させない。外れていても1タップで直せる
        #expect(slot(hour: 7) == .breakfast)
        #expect(slot(hour: 12) == .lunch)
        #expect(slot(hour: 19) == .dinner)
        #expect(slot(hour: 22) == .snack)
    }

    @Test("境界")
    func boundaries() {
        #expect(slot(hour: 4) == .snack)
        #expect(slot(hour: 5) == .breakfast)
        #expect(slot(hour: 10) == .breakfast)
        #expect(slot(hour: 11) == .lunch)
        #expect(slot(hour: 15) == .lunch)
        #expect(slot(hour: 16) == .dinner)
        #expect(slot(hour: 21) == .dinner)
    }

    @Test("openapi.yaml の値と一致する")
    func rawValues() {
        #expect(MealSlot.breakfast.rawValue == "朝食")
        #expect(MealSlot.lunch.rawValue == "昼食")
        #expect(MealSlot.dinner.rawValue == "夕食")
        #expect(MealSlot.snack.rawValue == "間食")
    }
}

@Suite("その日の合計")
struct MealTotalsTests {
    private func meal(_ kcal: Int?, _ p: Double?, _ f: Double?, _ c: Double?) -> Meal {
        Meal(
            id: UUID(), date: "2026-09-16", slot: .lunch, name: "x",
            qty: nil, kcal: kcal, proteinG: p, fatG: f, carbG: c, source: .manual
        )
    }

    @Test("PFC と kcal を足す")
    func sums() {
        let t = MealTotals(of: [meal(500, 30, 10, 60), meal(300, 20, 5, 40)])

        #expect(t.kcal == 800)
        #expect(t.proteinG == 50)
        #expect(t.fatG == 15)
        #expect(t.carbG == 100)
    }

    @Test("**未入力を 0 として扱わない**")
    func countsUnknown() {
        // 0 と混ぜると「食べたが記録が雑」と「食べていない」が同じに見える
        let t = MealTotals(of: [meal(500, 30, 10, 60), meal(nil, nil, nil, nil)])

        #expect(t.kcal == 500)
        #expect(t.withoutMacros == 1)
    }

    @Test("空なら 0")
    func empty() {
        let t = MealTotals(of: [])

        #expect(t.kcal == 0)
        #expect(t.withoutMacros == 0)
    }
}

@Suite("残量")
struct RemainingTests {
    @Test("目標から実績を引く")
    func remaining() {
        let target = Macros(kcal: 2400, proteinG: 180, fatG: 70, carbG: 250)
        let eaten = MealTotals(kcal: 900, proteinG: 60, fatG: 20, carbG: 100, withoutMacros: 0)

        let r = eaten.remaining(from: target)

        #expect(r.kcal == 1500)
        #expect(r.proteinG == 120)
    }

    @Test("超えたら負になる")
    func over() {
        let target = Macros(kcal: 2000, proteinG: 150, fatG: 60, carbG: 200)
        let eaten = MealTotals(kcal: 2200, proteinG: 160, fatG: 70, carbG: 210, withoutMacros: 0)

        // 0 で止めると「あとどれだけ削るか」が分からない
        #expect(eaten.remaining(from: target).kcal == -200)
    }
}

private let base = URL(string: "http://api.test")!

@Suite("食事の API")
struct MealAPITests {
    @Test("その日の食事を引く")
    func listMeals() async throws {
        let t = FakeTransport(json: """
        {"items":[{"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-16",
        "slot":"昼食","name":"鶏むね","kcal":330,"proteinG":62,"fatG":7,"carbG":0}]}
        """)
        let api = APIClient(baseURL: base, transport: t)

        let meals = try await api.listMeals(from: "2026-09-16", to: "2026-09-16")

        #expect(meals.count == 1)
        #expect(meals[0].slot == .lunch)
        #expect(meals[0].proteinG == 62)

        let req = try #require(t.requests.first)
        #expect(req.url?.path == "/v1/meals")
        #expect(req.url?.query?.contains("from=2026-09-16") == true)
    }

    @Test("記録する")
    func createMeal() async throws {
        let t = FakeTransport(json: """
        {"id":"11111111-1111-1111-1111-111111111111","date":"2026-09-16","name":"鶏むね","source":"manual"}
        """, status: 201)
        let api = APIClient(baseURL: base, transport: t)

        let input = MealInput(
            id: UUID(), date: "2026-09-16", slot: .lunch, name: "鶏むね",
            qty: "200g", kcal: 330, proteinG: 62, fatG: 7, carbG: 0, source: .manual
        )
        _ = try await api.createMeal(input)

        let req = try #require(t.requests.first)
        #expect(req.httpMethod == "POST")

        let body = try #require(req.httpBody)
        let sent = try JSONDecoder().decode(MealInput.self, from: body)
        #expect(sent.name == "鶏むね")
        // enum の値は日本語のまま送る（openapi.yaml と一致させる）
        #expect(sent.slot == .lunch)
    }

    @Test("候補を引く")
    func suggestions() async throws {
        let t = FakeTransport(json: """
        {"items":[{"name":"鶏むね","count":12,"kcal":330,"proteinG":62}]}
        """)
        let api = APIClient(baseURL: base, transport: t)

        let items = try await api.mealSuggestions(query: "鶏")

        #expect(items.first?.name == "鶏むね")
        #expect(items.first?.count == 12)
        #expect(t.requests.first?.url?.query?.contains("q=") == true)
    }

    @Test("その日の目標を引く")
    func targets() async throws {
        let t = FakeTransport(json: """
        {"date":"2026-09-16","target":{"kcal":2400,"proteinG":180,"fatG":70,"carbG":250},
         "consumed":{"kcal":0,"proteinG":0,"fatG":0,"carbG":0}}
        """)
        let api = APIClient(baseURL: base, transport: t)

        let targets = try await api.dailyTargets(date: "2026-09-16")

        #expect(targets.target?.kcal == 2400)
        #expect(t.requests.first?.url?.path == "/v1/targets/2026-09-16")
    }

    @Test("削除する")
    func deleteMeal() async throws {
        let t = FakeTransport(json: "", status: 204)
        let api = APIClient(baseURL: base, transport: t)
        let id = UUID()

        try await api.deleteMeal(id: id)

        let req = try #require(t.requests.first)
        #expect(req.httpMethod == "DELETE")
        #expect(req.url?.path == "/v1/meals/\(id.uuidString.lowercased())")
    }
}
