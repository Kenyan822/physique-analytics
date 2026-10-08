import Foundation
import Testing

@testable import PhysiqueCore

private let base = URL(string: "http://api.test")!

/// 2026-10 の一部。**判定できない日（null）を混ぜてある**
private let october = #"""
{"items":[
 {"date":"2026-10-01","mealGoalMet":true,"trained":true},
 {"date":"2026-10-02","mealGoalMet":true,"trained":false},
 {"date":"2026-10-03","mealGoalMet":false,"trained":true},
 {"date":"2026-10-04","mealGoalMet":null,"trained":false},
 {"date":"2026-10-05","mealGoalMet":true,"trained":true}]}
"""#

@Suite("ストリークの月表示")
@MainActor
struct StreakModelTests {
    private func loaded(_ json: String = october, today: String = "2026-10-08") async
        -> (StreakModel, FakeTransport)
    {
        let t = FakeTransport()
        t.responses = [(Data(json.utf8), 200)]
        let m = StreakModel(api: APIClient(baseURL: base, transport: t), today: today)
        await m.load()

        return (m, t)
    }

    @Test("その月の範囲を引く")
    func fetchesMonth() async {
        let (_, t) = await loaded()

        let url = try! #require(t.requests.first?.url)
        #expect(url.path == "/v1/streaks")
        #expect(url.query?.contains("from=2026-10-01") == true)
        #expect(url.query?.contains("to=2026-10-31") == true)
    }

    @Test("日付で引ける")
    func lookup() async {
        let (m, _) = await loaded()

        #expect(m.day("2026-10-01")?.trained == true)
        #expect(m.day("2026-10-03")?.mealGoalMet == false)
        // **判定できない日は nil。** 未達と混ぜない
        #expect(m.day("2026-10-04")?.mealGoalMet == nil)
        #expect(m.day("2026-10-09") == nil)
    }

    @Test("月の見出し")
    func title() async {
        let (m, _) = await loaded()

        #expect(m.monthLabel == "2026年10月")
    }

    @Test("**今日より先の月には進めない**")
    func cannotGoFuture() async {
        let (m, t) = await loaded(today: "2026-10-08")

        #expect(!m.canGoNext)

        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]
        await m.goToPreviousMonth()

        #expect(m.monthLabel == "2026年9月")
        #expect(m.canGoNext)
    }

    @Test("カレンダーの升目は日曜始まり")
    func grid() async {
        let (m, _) = await loaded()

        // 2026-10-01 は木曜。日曜始まりなら前に4つ空く
        #expect(m.leadingBlanks == 4)
        #expect(m.daysInMonth == 31)
    }

    @Test("**食事の連続日数**は今日から遡って数える")
    func mealStreak() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-05","mealGoalMet":true,"trained":false},
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":true,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 4)
    }

    @Test("**判定できない日で切れない**（未達ではないので飛ばす）")
    func nullDoesNotBreak() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":null,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 2)
    }

    @Test("未達で切れる")
    func falseBreaks() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":false,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 1)
    }
}
