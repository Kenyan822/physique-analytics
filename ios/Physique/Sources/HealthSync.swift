import Foundation

/// HealthKit から取り込む種類（要件 B-01 / B-09）。
enum HealthKind: String, CaseIterable, Sendable {
    case bodyMass
    case bodyFatPercentage
    case stepCount
    case sleepDuration
    // 以下は Apple Watch 装着時のみ（要件 B-09）
    case hrv
    case restingHeartRate
    case deepSleepDuration

    /// 同じ日に複数あるとき、合計するか最後の値を採るか。
    ///
    /// **歩数と睡眠は合計。** 区間ごとに記録されるので、合計しないと過少になる。
    /// **体重は最後。** 1日に何度も乗ることがあり、足すと無意味な値になる。
    var aggregation: Aggregation {
        switch self {
        case .stepCount, .sleepDuration, .deepSleepDuration: .sum
        case .bodyMass, .bodyFatPercentage, .hrv, .restingHeartRate: .last
        }
    }

    enum Aggregation: Sendable { case sum, last }
}

/// HealthKit の1サンプル。
struct HealthSample: Sendable, Hashable {
    let kind: HealthKind
    let date: Date
    /// HealthKit の既定単位での値（体重 kg / 睡眠と HRV は秒 / 心拍は bpm）
    let value: Double
}

/// HealthKit の読み取り口。
///
/// **インターフェースにしてあるのは、実機が無くても取り込みの判断を
/// テストできるようにするため。** HealthKit はシミュレータでは
/// データが空なので、本物では検証にならない。
protocol HealthSource: Sendable {
    func requestAuthorization() async throws
    func samples(for kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample]
}

/// HealthKit のサンプルを日次記録に畳む（要件 B-01 / B-09）。
enum HealthSync {
    /// 初回に取り込む日数。分析に要るのは直近30日（HRV の基準窓）。
    static let initialDays = 30

    /// サンプルを日ごとの入力にまとめる。
    ///
    /// 値が1つも無い日は作らない。**空の行を作ると「測ったが0だった」と
    /// 区別できなくなる。**
    static func toDaily(_ samples: [HealthSample]) -> [DailyMetricsInput] {
        var byDay: [String: [HealthKind: Double]] = [:]

        for s in samples.sorted(by: { $0.date < $1.date }) {
            let day = JST.dateString(from: s.date)
            var values = byDay[day] ?? [:]

            switch s.kind.aggregation {
            case .sum: values[s.kind] = (values[s.kind] ?? 0) + s.value
            case .last: values[s.kind] = s.value
            }
            byDay[day] = values
        }

        return byDay.keys.sorted().map { day in
            let v = byDay[day] ?? [:]
            var input = DailyMetricsInput(date: day)
            input.weightKg = v[.bodyMass]
            input.bodyfatPct = v[.bodyFatPercentage]
            input.steps = v[.stepCount].map { Int($0.rounded()) }
            // HealthKit は秒で返す。分析は時間・分・ms で扱う
            input.sleepH = v[.sleepDuration].map { ($0 / 3600).rounded(toPlaces: 1) }
            input.hrvMs = v[.hrv].map { Int(($0 * 1000).rounded()) }
            input.restingHr = v[.restingHeartRate].map { Int($0.rounded()) }
            input.deepSleepMin = v[.deepSleepDuration].map { Int(($0 / 60).rounded()) }

            return input
        }
    }

    /// 取り込みの開始日を決める。
    ///
    /// **毎回全期間を舐めない。** 最後に取り込んだ日の翌日から取る。
    /// 端末の時計がずれていても、今日より先は取りに行かない。
    static func syncFrom(lastSynced: String?, today: String) -> String {
        guard let lastSynced else {
            return shiftDays(today, -initialDays)
        }

        let next = shiftDays(lastSynced, 1)

        return next > today ? today : next
    }
}
