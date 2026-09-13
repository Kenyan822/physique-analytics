import Foundation

/// 体組成の入力画面の状態（要件 B-02 / B-03 / B-06）。
///
/// **前回値は初期値ではなく比較対象。** 前回の数字を入れておくと、
/// 測っていないのに測ったことになる。空のまま差分だけ見せる。
@Observable
@MainActor
final class BodyModel {
    private let api: APIClient

    /// 今日の体組成
    var weightKg: Double?
    var bodyfatPct: Double?
    private(set) var fatigue: Int?

    /// 今日の周囲長。触った箇所だけ入る
    var parts: [BodyPart: Double] = [:]

    /// 今日より前の直近の周囲長。差分の基準（要件 B-03）
    private(set) var previousMeasurement: BodyMeasurement?
    /// 今日より前の直近の体重
    private(set) var previousWeightKg: Double?

    private(set) var message: String?
    private(set) var isSaving = false

    /// HealthKit の読み取り口。nil なら取り込みボタンを出さない
    private let health: HealthSource?
    /// 最後に取り込んだ日。差分だけ取るために持つ
    private var lastSynced: String? {
        get { UserDefaults.standard.string(forKey: "healthLastSynced") }
        set { UserDefaults.standard.set(newValue, forKey: "healthLastSynced") }
    }

    var canSyncHealth: Bool { health != nil }

    init(api: APIClient, health: HealthSource? = nil) {
        self.api = api
        self.health = health
    }

    /// Apple Health から取り込む（要件 B-01 / B-09）。
    ///
    /// **手入力を上書きしない。** 送るのは HealthKit から来た項目だけで、
    /// 疲労度のような手入力の項目は触らない（API 側で nil は「変更しない」）。
    func syncHealth(today: String) async {
        guard let health else { return }

        isSaving = true
        message = nil
        defer { isSaving = false }

        do {
            try await health.requestAuthorization()

            let fromDay = HealthSync.syncFrom(lastSynced: lastSynced, today: today)
            guard let from = JST.date(from: fromDay), let to = JST.date(from: today) else {
                message = "日付を解釈できない"

                return
            }

            var samples: [HealthSample] = []
            for kind in HealthKind.allCases {
                // 一部の型だけ許可されないことがある。**そこで全部止めない**
                samples += (try? await health.samples(for: kind, from: from, to: to)) ?? []
            }

            let days = HealthSync.toDaily(samples)
            for day in days {
                _ = try await api.putDailyMetrics(day)
            }

            lastSynced = today
            message = days.isEmpty ? "取り込むものが無かった" : "\(days.count)日分を取り込んだ"
            await load(date: today)
        } catch {
            message = describe(error)
        }
    }

    /// その日の状態を読み込む。
    ///
    /// **今日すでに測っていれば、それを前回値にしない。** 同じ日を
    /// 比較対象にすると差分が常に 0 になる。
    func load(date: String) async {
        message = nil

        do {
            let daily = try await api.listDailyMetrics(from: shiftDays(date, -30), to: date)
            if let today = daily.first(where: { $0.date == date }) {
                weightKg = today.weightKg
                bodyfatPct = today.bodyfatPct
                fatigue = today.fatigue
            }
            previousWeightKg = daily.first { $0.date != date && $0.weightKg != nil }?.weightKg

            let latest = try await api.latestMeasurement()
            if let latest {
                if latest.date == date {
                    for part in BodyPart.allCases {
                        parts[part] = latest.value(for: part)
                    }
                } else {
                    previousMeasurement = latest
                }
            }
        } catch {
            message = describe(error)
        }
    }

    /// 疲労度を1タップで記録する（要件 B-06）。同じ値をもう一度押すと外れる。
    func setFatigue(_ value: Int) {
        fatigue = fatigue == value ? nil : value
    }

    /// 触った項目だけを入力にする。
    func measurementInput(date: String) -> BodyMeasurementInput {
        var input = BodyMeasurementInput(date: date)
        for (part, value) in parts {
            input.set(part, value)
        }

        return input
    }

    /// 体組成（体重・体脂肪率・疲労度）を保存する。
    func saveDaily(date: String) async {
        await save {
            _ = try await self.api.putDailyMetrics(DailyMetricsInput(
                date: date, weightKg: self.weightKg,
                bodyfatPct: self.bodyfatPct, fatigue: self.fatigue
            ))
        } success: { "体組成を保存した" }
    }

    /// 周囲長を保存する。触っていなければ何もしない。
    func saveMeasurement(date: String) async {
        guard !parts.isEmpty else {
            message = "周囲長がまだ入っていない"

            return
        }

        await save {
            _ = try await self.api.putMeasurement(self.measurementInput(date: date))
        } success: { "周囲長を保存した" }
    }

    private func save(_ work: () async throws -> Void, success: () -> String) async {
        isSaving = true
        message = nil
        defer { isSaving = false }

        do {
            try await work()
            message = success()
        } catch {
            message = describe(error)
        }
    }

    private func describe(_ error: Error) -> String {
        // API が返した理由をそのまま見せる。何が悪いか分からないと直せない
        if let e = error as? APIError { return e.errorDescription ?? "\(e)" }

        return error.localizedDescription
    }
}

/// YYYY-MM-DD を n 日ずらす。
///
/// 暦の計算は JST に寄せる（ADR-0013）。ここで別の Calendar を作ると、
/// 日付の境界が JST.dateString とずれる。
func shiftDays(_ date: String, _ n: Int) -> String {
    guard let d = JST.date(from: date),
          let shifted = JST.calendar.date(byAdding: .day, value: n, to: d)
    else { return date }

    return JST.dateString(from: shifted)
}
