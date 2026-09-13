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
