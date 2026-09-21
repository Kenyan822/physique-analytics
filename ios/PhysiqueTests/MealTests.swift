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

@Suite("記録に時刻を載せる")
struct MealTimeTests {
    @Test("JST の時刻を HH:mm で返す")
    func timeString() {
        var c = DateComponents()
        c.year = 2026; c.month = 9; c.day = 19; c.hour = 19; c.minute = 40
        let d = JST.calendar.date(from: c)!

        #expect(JST.timeString(from: d) == "19:40")
    }

    // **ホストのタイムゾーンで答えが変わらない**（#171 と同じ罠）
    @Test("UTC の時刻ではなく JST で出す")
    func timeStringIsJST() {
        // JST 8:00 = UTC 23:00（前日）
        #expect(JST.timeString(from: Date(timeIntervalSince1970: 1_789_772_400)) == "08:00")
    }

    @Test("0時台も2桁で揃える")
    func padsHour() {
        var c = DateComponents()
        c.year = 2026; c.month = 9; c.day = 19; c.hour = 3; c.minute = 5
        let d = JST.calendar.date(from: c)!

        #expect(JST.timeString(from: d) == "03:05")
    }

    @Test("**区分ではなく時刻を送る**")
    func inputCarriesTime() {
        var d = MealDraft()
        d.name = "鶏むね"

        let input = d.toInput(date: "2026-09-19", at: "19:40")

        #expect(input.at == "19:40")
        // 区分はサーバが導出する。クライアントは決めない（#191）
        #expect(input.slot == nil)
    }
}

@Suite("PFC 中心の入力")
struct MealDraftPFCTests {
    private func draft(p: String = "", f: String = "", c: String = "", name: String = "") -> MealDraft {
        var d = MealDraft()
        d.proteinG = p; d.fatG = f; d.carbG = c; d.name = name

        return d
    }

    @Test("**PFC だけで記録できる**")
    func pfcOnly() {
        let input = draft(p: "30", f: "10", c: "60").toInput(date: "2026-09-19", at: "19:40")

        #expect(input.name == nil)
        #expect(input.proteinG == 30)
        #expect(input.carbG == 60)
    }

    @Test("**kcal を送らない。** サーバが PFC から計算する")
    func doesNotSendKcal() {
        #expect(draft(p: "30", f: "10", c: "60").toInput(date: "d", at: "12:00").kcal == nil)
    }

    @Test("表示用の kcal はサーバと同じ式で出す")
    func showsKcal() {
        // Atwater 4/9/4。サーバの analytics.KcalFromMacros と一致させる
        #expect(draft(p: "30", f: "10", c: "60").kcal == 450)
        #expect(draft(p: "30.1", f: "10.2", c: "60.3").kcal == 453)
    }

    // **空欄を 0 とみなす。** 鶏むねの C のように「本当に 0」で
    // 空のまま済ませる場面が普通にある。全部揃うまで出さないと、
    // 爆速入力のつもりが「なぜ出ないのか」を考える時間になる
    @Test("**1つでも入っていれば残りは 0 として計算する**")
    func kcalWithBlanks() {
        #expect(draft(p: "30", f: "10").kcal == 30 * 4 + 10 * 9)
        #expect(draft(p: "45").kcal == 180)
        #expect(draft(c: "60").kcal == 240)
    }

    @Test("PFC が1つも無ければ kcal を出さない")
    func noKcalWhenNothingEntered() {
        #expect(draft().kcal == nil)
        // 名前だけの記録に 0 kcal を付けない
        #expect(draft(name: "外食").kcal == nil)
    }

    @Test("全部空なら記録しない")
    func emptyDraft() {
        #expect(draft().isEmpty)
    }

    @Test("**PFC のどれかが入っていれば記録できる**")
    func pfcMakesItRecordable() {
        #expect(!draft(p: "30").isEmpty)
        #expect(!draft(c: "60").isEmpty)
    }

    @Test("名前だけでも記録できる")
    func nameOnly() {
        let d = draft(name: "外食")
        #expect(!d.isEmpty)
        #expect(d.toInput(date: "d", at: "12:00").name == "外食")
    }

    @Test("空白だけの名前は送らない")
    func blankName() {
        #expect(draft(p: "1", name: "   ").toInput(date: "d", at: "12:00").name == nil)
    }
}

@Suite("時刻と Date の行き来")
struct JSTTimeRoundTripTests {
    @Test("HH:MM から Date にできる")
    func parses() {
        let d = try! #require(JST.time(from: "19:40"))
        let c = JST.calendar.dateComponents([.hour, .minute], from: d)

        #expect(c.hour == 19)
        #expect(c.minute == 40)
    }

    @Test("往復しても変わらない")
    func roundTrip() {
        for s in ["00:00", "07:05", "12:30", "23:59"] {
            let d = try! #require(JST.time(from: s))
            #expect(JST.timeString(from: d) == s)
        }
    }

    @Test("読めない値は nil")
    func invalid() {
        #expect(JST.time(from: "25:00") == nil)
        #expect(JST.time(from: "") == nil)
        #expect(JST.time(from: "1940") == nil)
    }
}

@Suite("食品マスタの展開")
struct FoodItemExpandTests {
    private func protein() -> FoodItem {
        FoodItem(
            id: UUID(), name: "プロテイン", qty: nil,
            proteinG: nil, fatG: nil, carbG: nil,
            components: [
                FoodItemComponent(name: "量", unit: "g", basisAmount: 30, defaultAmount: 30,
                                  proteinG: 24, fatG: 1.5, carbG: 2),
            ],
            usedCount: 0
        )
    }

    @Test("**引数が無ければそのまま返す**")
    func noComponents() {
        let egg = FoodItem(
            id: UUID(), name: "ゆで卵", qty: "1個",
            proteinG: 6.5, fatG: 5.2, carbG: 0.2, components: [], usedCount: 0
        )

        let got = egg.expand([:])

        #expect(got.proteinG == 6.5)
        #expect(got.carbG == 0.2)
    }

    @Test("引数も値も無ければ 0")
    func empty() {
        let x = FoodItem(id: UUID(), name: "x", qty: nil,
                         proteinG: nil, fatG: nil, carbG: nil, components: [], usedCount: 0)

        #expect(x.expand([:]).proteinG == 0)
    }

    @Test("基準量との比で計算する")
    func ratio() {
        let got = protein().expand(["量": 45])

        #expect(abs(got.proteinG - 36) < 0.001)
        #expect(abs(got.fatG - 2.25) < 0.001)
        #expect(abs(got.carbG - 3) < 0.001)
    }

    @Test("渡さなければ既定値を使う")
    func usesDefault() {
        let got = protein().expand([:])

        #expect(abs(got.proteinG - 24) < 0.001)
    }

    @Test("引数を複数持てる")
    func multiple() {
        let soboro = FoodItem(
            id: UUID(), name: "鶏そぼろ", qty: nil,
            proteinG: nil, fatG: nil, carbG: nil,
            components: [
                FoodItemComponent(name: "鶏ひき肉", unit: "g", basisAmount: 100,
                                  defaultAmount: 200, proteinG: 17.5, fatG: 12, carbG: 0),
                FoodItemComponent(name: "砂糖", unit: "g", basisAmount: 100,
                                  defaultAmount: 10, proteinG: 0, fatG: 0, carbG: 99.2),
            ],
            usedCount: 0
        )

        // 肉だけ 230g に変える。砂糖は既定の 10g
        let got = soboro.expand(["鶏ひき肉": 230])

        #expect(abs(got.proteinG - 17.5 * 2.3) < 0.001)
        #expect(abs(got.carbG - 9.92) < 0.001)
    }

    @Test("**0 を渡せる。** 既定値に戻さない")
    func zero() {
        #expect(protein().expand(["量": 0]).proteinG == 0)
    }

    @Test("基準量が 0 なら 0 扱い")
    func zeroBasis() {
        let broken = FoodItem(
            id: UUID(), name: "x", qty: nil, proteinG: nil, fatG: nil, carbG: nil,
            components: [FoodItemComponent(name: "量", unit: "g", basisAmount: 0,
                                           defaultAmount: 10, proteinG: 5, fatG: 0, carbG: 0)],
            usedCount: 0
        )

        #expect(broken.expand([:]).proteinG == 0)
    }

    @Test("既定値を取り出せる")
    func defaults() {
        #expect(protein().defaultAmounts == ["量": 30])
        let egg = FoodItem(id: UUID(), name: "卵", qty: nil, proteinG: 6, fatG: 5, carbG: 0,
                           components: [], usedCount: 0)
        #expect(egg.defaultAmounts.isEmpty)
    }
}

/// **サーバと同じ結果になることを固定する。**
///
/// 期待値は `internal/foodmaster.Expand` に同じ入力を与えて出したもの。
/// 片方だけ式を変えたら、ここが落ちる。
///
/// 食い違うと、**画面に出ていた値が記録した瞬間に変わる**。
@Suite("サーバと同じ計算になる")
struct FoodItemParityTests {
    private let protein = FoodItem(
        id: UUID(), name: "プロテイン", qty: nil,
        proteinG: nil, fatG: nil, carbG: nil,
        components: [
            FoodItemComponent(name: "量", unit: "g", basisAmount: 30, defaultAmount: 30,
                              proteinG: 24, fatG: 1.5, carbG: 2),
        ],
        usedCount: 0
    )

    private let soboro = FoodItem(
        id: UUID(), name: "鶏そぼろ", qty: nil,
        proteinG: nil, fatG: nil, carbG: nil,
        components: [
            FoodItemComponent(name: "鶏ひき肉", unit: "g", basisAmount: 100, defaultAmount: 200,
                              proteinG: 17.5, fatG: 12, carbG: 0),
            FoodItemComponent(name: "砂糖", unit: "g", basisAmount: 100, defaultAmount: 10,
                              proteinG: 0, fatG: 0, carbG: 99.2),
        ],
        usedCount: 0
    )

    @Test("プロテイン 45g")
    func protein45() { assertSame(protein.expand(["量": 45]), 36.0, 2.25, 3.0) }

    @Test("プロテイン 既定（30g）")
    func proteinDefault() { assertSame(protein.expand([:]), 24.0, 1.5, 2.0) }

    @Test("プロテイン 0g")
    func proteinZero() { assertSame(protein.expand(["量": 0]), 0, 0, 0) }

    @Test("そぼろ 肉 230g・砂糖は既定")
    func soboro230() { assertSame(soboro.expand(["鶏ひき肉": 230]), 40.25, 27.6, 9.92) }

    // ---- #218: 本体と引数の足し算 ----

    /// 本体が量に比例する形（引数の行を作らない）
    private let scaled = FoodItem(
        id: UUID(), name: "プロテイン", qty: nil,
        proteinG: 24, fatG: 1.5, carbG: 2,
        baseAmount: 30, baseUnit: "g", scalesWithAmount: true,
        components: [], usedCount: 0
    )

    /// 固定部分＋量が変わる部分
    private let teishoku = FoodItem(
        id: UUID(), name: "定食", qty: nil,
        proteinG: 20, fatG: 10, carbG: 80,
        components: [
            FoodItemComponent(name: "鶏むね", unit: "g", basisAmount: 100, defaultAmount: 200,
                              proteinG: 23, fatG: 1.9, carbG: 0),
        ],
        usedCount: 0
    )

    @Test("比例する本体 45g")
    func scaled45() { assertSame(scaled.expand(base: 45), 36.0, 2.25, 3.0) }

    @Test("比例する本体 量を渡さなければ基準量ぶん")
    func scaledDefault() { assertSame(scaled.expand(), 24.0, 1.5, 2.0) }

    @Test("**本体と引数を足す**")
    func basePlusComponents() {
        // 20 + 46 / 10 + 3.8 / 80 + 0
        assertSame(teishoku.expand(), 66.0, 13.8, 80.0)
    }

    @Test("比例しないなら入力量を無視する")
    func ignoresAmountWhenNotScaling() {
        assertSame(teishoku.expand(base: 999), 66.0, 13.8, 80.0)
    }

    // **既存の登録の結果が変わらないことを固定する**（#218 の一番の関心事）
    @Test("既存: 引数なしは従来どおり")
    func legacyNoComponents() {
        let egg = FoodItem(id: UUID(), name: "ゆで卵", qty: nil,
                           proteinG: 6.5, fatG: 5.2, carbG: 0.2,
                           components: [], usedCount: 0)

        assertSame(egg.expand(), 6.5, 5.2, 0.2)
    }

    @Test("既存: 引数ありは従来どおり")
    func legacyWithComponents() {
        assertSame(protein.expand(["量": 45]), 36.0, 2.25, 3.0)
    }

    private func assertSame(_ got: Macros, _ p: Double, _ f: Double, _ c: Double) {
        #expect(abs(got.proteinG - p) < 0.0001)
        #expect(abs(got.fatG - f) < 0.0001)
        #expect(abs(got.carbG - c) < 0.0001)
    }
}
