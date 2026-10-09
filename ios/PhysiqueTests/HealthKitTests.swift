import Foundation
import Testing

@testable import PhysiqueCore

@Suite("HealthKit の取り込み")
struct HealthSyncTests {
    /// HealthKit の代わりに固定の値を返す。
    /// **実機が無くても取り込みの判断をテストできるようにする。**
    struct FakeSource: HealthSource {
        var samples: [HealthSample] = []
        var authorized = true
        var authorizeError: Error?

        func requestAuthorization() async throws {
            if let authorizeError { throw authorizeError }
        }

        func samples(for kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample] {
            samples.filter { $0.kind == kind && $0.date >= from && $0.date <= to }
        }
    }

    private func day(_ d: Int) -> Date {
        JST.date(from: "2026-11-\(String(format: "%02d", d))")!
    }

    @Test("日ごとにまとめて DailyMetricsInput にする")
    func groupsByDay() {
        let samples = [
            HealthSample(kind: .bodyMass, date: day(1), value: 75.2),
            HealthSample(kind: .bodyFatPercentage, date: day(1), value: 19.8),
            HealthSample(kind: .stepCount, date: day(1), value: 8500),
            HealthSample(kind: .bodyMass, date: day(2), value: 75.0),
        ]

        let got = HealthSync.toDaily(samples)

        #expect(got.count == 2)
        let first = got.first { $0.date == "2026-11-01" }
        #expect(first?.weightKg == 75.2)
        #expect(first?.bodyfatPct == 19.8)
        #expect(first?.steps == 8500)
    }

    @Test("同じ日に複数あれば最後のものを使う")
    func lastWins() {
        // 体重は1日に何度も乗ることがある。**最後に測ったものを採る**
        let samples = [
            HealthSample(kind: .bodyMass, date: day(1), value: 75.5),
            HealthSample(kind: .bodyMass, date: day(1).addingTimeInterval(3600), value: 75.2),
        ]

        #expect(HealthSync.toDaily(samples).first?.weightKg == 75.2)
    }

    @Test("歩数は合計する")
    func stepsAreSummed() {
        // 歩数は区間ごとに記録される。**合計しないと過少になる**
        let samples = [
            HealthSample(kind: .stepCount, date: day(1), value: 3000),
            HealthSample(kind: .stepCount, date: day(1).addingTimeInterval(3600), value: 5500),
        ]

        #expect(HealthSync.toDaily(samples).first?.steps == 8500)
    }

    @Test("睡眠は時間に直す")
    func sleepToHours() {
        // HealthKit は秒で返す
        let samples = [HealthSample(kind: .sleepDuration, date: day(1), value: 7.5 * 3600)]

        #expect(HealthSync.toDaily(samples).first?.sleepH == 7.5)
    }

    @Test("Watch の指標も取り込む")
    func watchMetrics() {
        let samples = [
            HealthSample(kind: .hrv, date: day(1), value: 0.068),      // 秒で返る
            HealthSample(kind: .restingHeartRate, date: day(1), value: 52),
            HealthSample(kind: .deepSleepDuration, date: day(1), value: 72 * 60),
        ]

        let got = HealthSync.toDaily(samples).first
        // HRV は ms に直す（分析の閾値が ms）
        #expect(got?.hrvMs == 68)
        #expect(got?.restingHr == 52)
        #expect(got?.deepSleepMin == 72)
    }

    @Test("値が無い日は作らない")
    func noEmptyDays() {
        #expect(HealthSync.toDaily([]).isEmpty)
    }

    @Test("取り込む期間は最後に取り込んだ日の翌日から")
    func incrementalRange() {
        let from = HealthSync.syncFrom(lastSynced: "2026-10-28", today: "2026-11-01")

        // **毎回全期間を舐めない。** 差分だけ取る
        #expect(from == "2026-10-29")
    }

    @Test("初回は30日前から")
    func firstSync() {
        // 初回に全期間を取ると時間がかかる。分析に要るのは直近30日
        #expect(HealthSync.syncFrom(lastSynced: nil, today: "2026-11-01") == "2026-10-02")
    }

    @Test("未来の日付は取り込まない")
    func noFutureDays() {
        let from = HealthSync.syncFrom(lastSynced: "2026-11-05", today: "2026-11-01")

        // 端末の時計がずれていても、今日より先は取りに行かない
        #expect(from == "2026-11-01")
    }
}

/// 一度許可したら自動で取り込む（#274）。
@Suite("Apple Health の自動取り込み")
@MainActor
struct AutoSyncTests {
    /// 呼ばれた回数を数える偽物
    final class CountingSource: HealthSource, @unchecked Sendable {
        init() {}

        var authCalls = 0
        var sampleCalls = 0
        var shouldThrow: Error?

        func requestAuthorization() async throws {
            authCalls += 1
            if let shouldThrow { throw shouldThrow }
        }

        /// 最後に要求された開始日。取り込み直しの確認に使う
        var lastFrom: Date?

        func samples(for kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample] {
            sampleCalls += 1
            lastFrom = from

            return kind == .bodyMass
                ? [HealthSample(kind: .bodyMass, date: Date(), value: 72.4)]
                : []
        }
    }

    /// **テストごとに別の置き場所を使う。** `.standard` を共有すると汚し合う
    /// `putDailyMetrics` は `DailyMetrics` を期待する。**id が無いと decode で落ち**、
    /// catch に入って「取り込んだ」ではなくエラーが message に入る
    private func model(_ h: CountingSource, granted: Bool) -> BodyModel {
        let ok = #"{"id":"33333333-3333-4333-8333-333333333333","date":"2026-10-09"}"#
        let t = FakeTransport(json: ok)
        let suite = UserDefaults(suiteName: "test-\(UUID().uuidString)")!
        let m = BodyModel(api: APIClient(baseURL: URL(string: "http://api.test")!, transport: t),
                          health: h, defaults: suite)
        m.resetHealthPermissionForTesting(granted: granted)

        return m
    }

    @Test("**まだ許可していなければ自動では走らない**")
    func doesNotSyncBeforeGrant() async {
        let h = CountingSource()
        let m = model(h, granted: false)

        await m.syncHealthIfGranted(today: "2026-10-09")

        // 起動のたびに権限ダイアログが出るのを避ける
        #expect(h.authCalls == 0)
        #expect(h.sampleCalls == 0)
    }

    @Test("**一度許可したら自動で走る**")
    func syncsAfterGrant() async {
        let h = CountingSource()
        let m = model(h, granted: true)

        await m.syncHealthIfGranted(today: "2026-10-09")

        #expect(h.sampleCalls > 0)
    }

    @Test("手動で取り込むと以後は許可済みになる")
    func manualGrants() async {
        let h = CountingSource()
        let m = model(h, granted: false)

        await m.syncHealth(today: "2026-10-09")

        #expect(h.authCalls == 1)
        #expect(m.healthGranted)
    }

    @Test("**拒否されたら許可済みにしない**")
    func deniedDoesNotGrant() async {
        let h = CountingSource()
        h.shouldThrow = URLError(.notConnectedToInternet)
        let m = model(h, granted: false)

        await m.syncHealth(today: "2026-10-09")

        #expect(!m.healthGranted)
    }

    @Test("**手動で押したら結果が残る**")
    func manualShowsMessage() async {
        let h = CountingSource()
        let m = model(h, granted: false)

        await m.syncHealth(today: "2026-10-09")

        // syncHealth の最後で load() を呼ぶが、load は message = nil する。
        // **結果が消えるので、押しても何も起きないように見える**（実機で踏んだ）。
        // 「nil でない」だけでは足りない —— load の失敗メッセージでも通ってしまう
        #expect(m.message?.contains("取り込") == true)
    }

    @Test("**自動のときはメッセージを出さない**")
    func quietWhenAutomatic() async {
        let h = CountingSource()
        let m = model(h, granted: true)

        await m.syncHealthIfGranted(today: "2026-10-09")

        // 開くたびに「3日分を取り込んだ」と出ると邪魔。
        // **読み込みの失敗は出してよい**ので、取り込み結果だけ抑える
        #expect(m.message?.contains("取り込") != true)
    }
}

/// 体組成の日付移動を食事・記録と同じ形にする（#275）。
@Suite("体組成の日付移動")
@MainActor
struct BodyDateNavTests {
    private func model(date: String = "2026-10-09", today: String = "2026-10-09") -> BodyModel {
        let t = FakeTransport(json: #"{"date":"2026-10-09"}"#)
        return BodyModel(api: APIClient(baseURL: URL(string: "http://api.test")!, transport: t),
                         defaults: UserDefaults(suiteName: "t-\(UUID().uuidString)")!,
                         date: date, today: today)
    }

    @Test("曜日つきで表示する")
    func label() {
        #expect(model(date: "2026-10-09").dateLabel == "10/9(金)")
    }

    @Test("前の日に移れる")
    func previous() async {
        let m = model()

        await m.goToPreviousDay()

        #expect(m.date == "2026-10-08")
    }

    @Test("**今日より先には進めない**")
    func noFuture() async {
        let m = model(date: "2026-10-09", today: "2026-10-09")
        #expect(!m.canGoNext)

        await m.goToNextDay()

        #expect(m.date == "2026-10-09")
    }

    @Test("戻ったら進めるようになる")
    func nextAfterBack() async {
        let m = model()
        await m.goToPreviousDay()

        #expect(m.canGoNext)
        await m.goToNextDay()
        #expect(m.date == "2026-10-09")
    }

    @Test("未来を選んだら今日に丸める")
    func clampsFuture() async {
        let m = model()

        await m.goTo("2026-12-31")

        #expect(m.date == "2026-10-09")
    }
}

@Suite("体組成の数値表示")
struct BodyNumberTextTests {
    @Test("小数第1位までにする")
    func rounds() {
        // HealthKit の値は 72.40000000000001 のように来る
        #expect(bodyText(72.40000000000001) == "72.4")
        #expect(bodyText(13.249999999) == "13.2")
    }

    @Test("整数は小数点を出さない")
    func integer() {
        #expect(bodyText(72) == "72")
        #expect(bodyText(72.0) == "72")
    }

    @Test("第2位以下は丸める")
    func truncates() {
        #expect(bodyText(72.46) == "72.5")
    }
}

/// 体組成の読み込み（#291）。
@Suite("体組成の前回値と日付移動")
@MainActor
struct BodyLoadTests {
    /// 10/8 と 10/5 に記録がある。order by date desc
    private let daily = #"""
    {"items":[
     {"id":"11111111-1111-4111-8111-111111111111","date":"2026-10-08","weightKg":78.4,"bodyfatPct":18.2},
     {"id":"22222222-2222-4222-8222-222222222222","date":"2026-10-05","weightKg":79.2,"bodyfatPct":18.9}]}
    """#

    private func model(date: String) -> (BodyModel, FakeTransport) {
        let t = FakeTransport()
        let m = BodyModel(api: APIClient(baseURL: URL(string: "http://api.test")!, transport: t),
                          defaults: UserDefaults(suiteName: "b-\(UUID().uuidString)")!,
                          date: date, today: "2026-10-09")

        return (m, t)
    }

    @Test("**体脂肪率にも前回値が出る**")
    func bodyfatPrevious() async {
        let (m, t) = model(date: "2026-10-08")
        t.responses = [(Data(daily.utf8), 200), (Data("null".utf8), 200)]

        await m.load(date: "2026-10-08")

        #expect(m.bodyfatPct == 18.2)
        #expect(m.previousBodyfatPct == 18.9)
    }

    @Test("前回は表示日より前の直近")
    func previousIsBeforeDate() async {
        let (m, t) = model(date: "2026-10-08")
        t.responses = [(Data(daily.utf8), 200), (Data("null".utf8), 200)]

        await m.load(date: "2026-10-08")

        #expect(m.previousWeightKg == 79.2)
    }

    @Test("**記録が無い日に移ると空欄になる**")
    func clearsOnEmptyDay() async {
        let (m, t) = model(date: "2026-10-08")
        t.responses = [(Data(daily.utf8), 200), (Data("null".utf8), 200)]
        await m.load(date: "2026-10-08")
        #expect(m.weightKg == 78.4)

        // 10/7 に記録は無い
        t.responses = [(Data(daily.utf8), 200), (Data("null".utf8), 200)]
        await m.load(date: "2026-10-07")

        #expect(m.weightKg == nil)
        #expect(m.bodyfatPct == nil)
        // その日より前の直近が前回になる
        #expect(m.previousWeightKg == 79.2)
    }

    @Test("**周囲長の前回は表示日より前**")
    func measurementBeforeDate() async {
        let (m, t) = model(date: "2026-10-06")
        let future = #"{"id":"33333333-3333-4333-8333-333333333333","date":"2026-10-08","neckCm":38}"#
        t.responses = [(Data(daily.utf8), 200), (Data(future.utf8), 200)]

        await m.load(date: "2026-10-06")

        // 10/8 は表示日より後。前回として出してはいけない
        #expect(m.previousMeasurement == nil)
    }
}

@Suite("取り込み直し")
@MainActor
struct ResyncTests {
    @Test("**押せば最初から取り込み直せる**")
    func resync() async {
        let t = FakeTransport(json: #"{"id":"44444444-4444-4444-8444-444444444444","date":"2026-10-09"}"#)
        let h = AutoSyncTests.CountingSource()
        let m = BodyModel(api: APIClient(baseURL: URL(string: "http://api.test")!, transport: t),
                          health: h,
                          defaults: UserDefaults(suiteName: "r-\(UUID().uuidString)")!)

        // 初回は30日前から
        await m.syncHealth(today: "2026-10-09")
        let first = try! #require(h.lastFrom)

        // 2回目は差分だけなので、もっと後ろから
        await m.syncHealth(today: "2026-10-09")
        let second = try! #require(h.lastFrom)
        #expect(second > first)

        // 取り込み直しは初回と同じところまで戻る
        await m.resyncHealth(today: "2026-10-09")
        #expect(h.lastFrom == first)
    }
}
