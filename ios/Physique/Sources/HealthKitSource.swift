#if canImport(HealthKit)
import Foundation
import HealthKit

/// HealthKit から実際に読む実装（要件 B-01 / B-09）。
///
/// **シミュレータでは検証にならない。** HealthKit のデータが空なので、
/// 取り込みの判断は `HealthSync` 側でテストしてある（`HealthSource` の
/// 偽物を使う）。ここは HealthKit の API を呼ぶだけに留める。
///
/// 読み取り専用。**書き込みの権限は要求しない**。このアプリが
/// Apple Health を汚す理由が無い。
struct HealthKitSource: HealthSource {
    private let store = HKHealthStore()

    /// 読み取る型。要求する権限はここに出ているものだけ。
    private static let readTypes: Set<HKObjectType> = {
        var types: Set<HKObjectType> = []
        for kind in HealthKind.allCases {
            if let t = objectType(for: kind) {
                types.insert(t)
            }
        }

        return types
    }()

    func requestAuthorization() async throws {
        guard HKHealthStore.isHealthDataAvailable() else {
            throw HealthError.unavailable
        }

        try await store.requestAuthorization(toShare: [], read: Self.readTypes)
    }

    func samples(for kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample] {
        switch kind {
        case .sleepDuration, .deepSleepDuration:
            try await sleepSamples(kind: kind, from: from, to: to)
        default:
            try await quantitySamples(kind: kind, from: from, to: to)
        }
    }

    private func quantitySamples(kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample] {
        guard let type = Self.quantityType(for: kind), let unit = Self.unit(for: kind) else {
            return []
        }

        let raw = try await query(type: type, from: from, to: to)

        return raw.compactMap { s in
            guard let q = s as? HKQuantitySample else { return nil }

            return HealthSample(kind: kind, date: q.endDate, value: q.quantity.doubleValue(for: unit))
        }
    }

    /// 睡眠は区間（開始〜終了）で入るので、長さを秒にして返す。
    private func sleepSamples(kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample] {
        guard let type = HKObjectType.categoryType(forIdentifier: .sleepAnalysis) else {
            return []
        }

        let raw = try await query(type: type, from: from, to: to)

        return raw.compactMap { s in
            guard let c = s as? HKCategorySample else { return nil }
            guard Self.matchesSleepStage(c, kind: kind) else { return nil }

            let seconds = c.endDate.timeIntervalSince(c.startDate)

            return HealthSample(kind: kind, date: c.endDate, value: seconds)
        }
    }

    /// 深睡眠だけを取るか、眠っている時間すべてを取るかを分ける。
    private static func matchesSleepStage(_ sample: HKCategorySample, kind: HealthKind) -> Bool {
        guard let value = HKCategoryValueSleepAnalysis(rawValue: sample.value) else { return false }

        switch kind {
        case .deepSleepDuration:
            return value == .asleepDeep
        default:
            // **inBed を含めない。** ベッドにいた時間は睡眠時間ではない
            return value == .asleepCore || value == .asleepDeep
                || value == .asleepREM || value == .asleepUnspecified
        }
    }

    private func query(type: HKObjectType, from: Date, to: Date) async throws -> [HKSample] {
        guard let sampleType = type as? HKSampleType else { return [] }

        let predicate = HKQuery.predicateForSamples(withStart: from, end: to, options: .strictEndDate)
        let sort = [NSSortDescriptor(key: HKSampleSortIdentifierEndDate, ascending: true)]

        return try await withCheckedThrowingContinuation { continuation in
            let q = HKSampleQuery(
                sampleType: sampleType, predicate: predicate,
                limit: HKObjectQueryNoLimit, sortDescriptors: sort
            ) { _, samples, error in
                if let error {
                    continuation.resume(throwing: error)

                    return
                }
                continuation.resume(returning: samples ?? [])
            }
            store.execute(q)
        }
    }

    private static func objectType(for kind: HealthKind) -> HKObjectType? {
        switch kind {
        case .sleepDuration, .deepSleepDuration:
            HKObjectType.categoryType(forIdentifier: .sleepAnalysis)
        default:
            quantityType(for: kind)
        }
    }

    private static func quantityType(for kind: HealthKind) -> HKQuantityType? {
        let id: HKQuantityTypeIdentifier? = switch kind {
        case .bodyMass: .bodyMass
        case .bodyFatPercentage: .bodyFatPercentage
        case .stepCount: .stepCount
        case .hrv: .heartRateVariabilitySDNN
        case .restingHeartRate: .restingHeartRate
        case .sleepDuration, .deepSleepDuration: nil
        }

        return id.flatMap { HKObjectType.quantityType(forIdentifier: $0) }
    }

    /// 取り出す単位。**ここを間違えると静かに桁がずれる。**
    private static func unit(for kind: HealthKind) -> HKUnit? {
        switch kind {
        case .bodyMass: .gramUnit(with: .kilo)
        case .bodyFatPercentage: .percent()
        case .stepCount: .count()
        // SDNN は秒で取り、HealthSync が ms に直す
        case .hrv: .second()
        case .restingHeartRate: HKUnit.count().unitDivided(by: .minute())
        case .sleepDuration, .deepSleepDuration: nil
        }
    }
}

enum HealthError: Error, LocalizedError {
    case unavailable

    var errorDescription: String? {
        switch self {
        case .unavailable:
            "この端末では Apple Health が使えない"
        }
    }
}
#endif
