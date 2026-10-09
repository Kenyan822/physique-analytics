import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

/// 未達の日。理由（何がどれだけ足りないか）まで返る
private let missed = #"""
{"date":"2026-10-03",
 "meals":{"consumed":{"kcal":1820,"proteinG":150,"fatG":60,"carbG":200},
          "target":{"kcal":2110,"proteinG":180,"fatG":70,"carbG":250},
          "goalMet":false,
          "shortfall":{"kcal":-290,"proteinG":-30,"fatG":-10,"carbG":-50}},
 "workout":{"templateName":"胸","dayOrder":1,"setCount":12,
            "exercises":[{"exerciseName":"ベンチプレス","setCount":5,"topWeightKg":80}]},
 "body":{"weightKg":72.4,"bodyFatPct":13.2}}
"""#

/// 記録が無い日
private let empty = #"""
{"date":"2026-10-04","meals":{"consumed":{"kcal":0,"proteinG":0,"fatG":0,"carbG":0}}}
"""#

@Suite("その日の詳細")
struct DayDetailTests {
    @Test("GET /v1/days/{date} を叩く")
    func fetches() async throws {
        let t = FakeTransport(json: missed)
        let api = APIClient(baseURL: base, transport: t)

        let got = try await api.day("2026-10-03")

        #expect(got.date == "2026-10-03")
        #expect(try #require(t.requests.last?.url).path == "/v1/days/2026-10-03")
    }

    @Test("食事の実績と目標が読める")
    func meals() async throws {
        let got = try await APIClient(baseURL: base, transport: FakeTransport(json: missed))
            .day("2026-10-03")

        #expect(got.meals.consumed.proteinG == 150)
        #expect(got.meals.target?.proteinG == 180)
        #expect(got.meals.goalMet == false)
    }

    @Test("**未達の理由が出る**")
    func reason() async throws {
        let got = try await APIClient(baseURL: base, transport: FakeTransport(json: missed))
            .day("2026-10-03")

        // 足りない順に並べる。一番効いているものから直したい
        #expect(got.meals.reasons == ["C が 50g 足りない", "P が 30g 足りない", "F が 10g 足りない"])
    }

    @Test("筋トレと体組成が読める")
    func workoutAndBody() async throws {
        let got = try await APIClient(baseURL: base, transport: FakeTransport(json: missed))
            .day("2026-10-03")

        #expect(got.workout?.templateName == "胸")
        #expect(got.workout?.setCount == 12)
        #expect(got.body?.weightKg == 72.4)
    }

    @Test("**記録が無い日でも落ちない**")
    func emptyDay() async throws {
        let got = try await APIClient(baseURL: base, transport: FakeTransport(json: empty))
            .day("2026-10-04")

        #expect(got.workout == nil)
        #expect(got.body == nil)
        #expect(got.meals.target == nil)
        #expect(got.meals.goalMet == nil)
        // 目標が無ければ理由も出さない。「未達」と「判定できない」を混ぜない
        #expect(got.meals.reasons.isEmpty)
    }

    @Test("達成した日は理由を出さない")
    func metDay() async throws {
        let json = #"""
        {"date":"2026-10-05","meals":{"consumed":{"kcal":2100,"proteinG":185,"fatG":72,"carbG":240},
         "target":{"kcal":2110,"proteinG":180,"fatG":70,"carbG":250},"goalMet":true}}
        """#
        let got = try await APIClient(baseURL: base, transport: FakeTransport(json: json))
            .day("2026-10-05")

        #expect(got.meals.reasons.isEmpty)
    }
}
