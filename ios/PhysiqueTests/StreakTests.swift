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

    /// #348。**一日の途中は当たり前に未達。** 食べ終わるまで 0 に見えると、
    /// 続けること自体が目的の表示なのに効き目が無くなる
    @Test("**今日がまだでも昨日までの連続が出る**")
    func todayPendingKeepsStreak() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-05","mealGoalMet":true,"trained":false},
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":true,"trained":false},
         {"date":"2026-10-08","mealGoalMet":false,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 3)
    }

    /// **今日を達成したら足す。** 昨日までが2日なら3日になる
    @Test("**今日を達成すると1増える**")
    func todayDoneAddsOne() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":true,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 3)
    }

    /// **昨日で切れていれば、今日を達成しても1**
    @Test("**昨日が未達なら今日を達成しても1**")
    func yesterdayBrokenGivesOne() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-06","mealGoalMet":true,"trained":false},
         {"date":"2026-10-07","mealGoalMet":false,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 1)
    }

    /// 筋トレ側にも同じく効く
    @Test("**筋トレも今日がまだでは切れない**")
    func trainedTodayPending() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-06","mealGoalMet":null,"trained":true},
         {"date":"2026-10-07","mealGoalMet":null,"trained":true},
         {"date":"2026-10-08","mealGoalMet":null,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.trainedStreak == 2)
    }

    /// **過去の未達では切れる。** 切れないのは「今日がまだ」のときだけ
    @Test("未達で切れる")
    func falseBreaks() async {
        let run = #"""
        {"items":[
         {"date":"2026-10-05","mealGoalMet":true,"trained":false},
         {"date":"2026-10-06","mealGoalMet":false,"trained":false},
         {"date":"2026-10-07","mealGoalMet":true,"trained":false},
         {"date":"2026-10-08","mealGoalMet":true,"trained":false}]}
        """#
        let (m, _) = await loaded(run, today: "2026-10-08")

        #expect(m.mealStreak == 2)
    }
}

/// カレンダーの日をタップして中身を見る（#276）。
@Suite("日をタップして開く")
@MainActor
struct DayPickTests {
    private let detail = #"""
    {"date":"2026-10-03",
     "meals":{"consumed":{"kcal":1820,"proteinG":150,"fatG":60,"carbG":200},
              "target":{"kcal":2110,"proteinG":180,"fatG":70,"carbG":250},
              "goalMet":false,
              "shortfall":{"kcal":-290,"proteinG":-30,"fatG":-10,"carbG":-50}}}
    """#

    private func loaded() async -> (StreakModel, FakeTransport) {
        let t = FakeTransport()
        t.responses = [(Data(#"{"items":[]}"#.utf8), 200)]
        let m = StreakModel(api: APIClient(baseURL: base, transport: t), today: "2026-10-09")
        await m.load()

        return (m, t)
    }

    @Test("日を選ぶとその日を引く")
    func picks() async {
        let (m, t) = await loaded()
        t.responses = [(Data(detail.utf8), 200)]

        await m.pick("2026-10-03")

        #expect(m.picked?.date == "2026-10-03")
        #expect(try! #require(t.requests.last?.url).path == "/v1/days/2026-10-03")
    }

    @Test("**未来の日は開かない**")
    func noFuture() async {
        let (m, t) = await loaded()
        let n = t.requests.count

        await m.pick("2026-10-20")

        #expect(t.requests.count == n)
        #expect(m.picked == nil)
    }

    @Test("閉じられる")
    func closes() async {
        let (m, t) = await loaded()
        t.responses = [(Data(detail.utf8), 200)]
        await m.pick("2026-10-03")

        m.closePicked()

        #expect(m.picked == nil)
    }

    @Test("**失敗しても画面は閉じない**")
    func keepsOpenOnFailure() async {
        let (m, t) = await loaded()
        t.responses = [(Data(#"{"type":"about:blank","title":"無い","status":404}"#.utf8), 404)]

        await m.pick("2026-10-03")

        #expect(m.picked == nil)
        #expect(m.showError)
    }
}
