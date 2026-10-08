import Foundation

/// 今日やる想定（要件 T-01 / #232）。
///
/// **`days` が空ならルーティン未登録。** そのときも記録はできる。
struct TodayRoutine: Codable, Sendable {
    let date: String
    var routineName: String?
    /// 全 Day。**手でずらせるように全部返る**（6日 × 4種目で数 KB）
    var days: [RoutineDay]?
    /// 今日やる Day の `dayOrder`。**添字ではない**（番号は飛びうる）
    var todayOrder: Int?

    /// 今日やる Day
    var today: RoutineDay? {
        guard let order = todayOrder else { return nil }

        return days?.first { $0.dayOrder == order }
    }

    var totalDays: Int { days?.count ?? 0 }
}

/// ルーティンの1日ぶん。
struct RoutineDay: Codable, Sendable, Identifiable {
    var id: UUID { templateId }

    /// 巡回の順。**連番とは限らない**（途中の日を消せる）
    let dayOrder: Int
    let templateId: UUID
    let templateName: String
    var items: [RoutineDayItem]
}

/// 今日やる種目1つ。**前回の実施内容を含む。**
///
/// 行ごとに前回値を引くと、画面を開くたびに種目数ぶんの往復になる。
struct RoutineDayItem: Codable, Sendable, Identifiable {
    var id: UUID { exerciseId }

    let exerciseId: UUID
    let exerciseName: String
    let muscleGroup: String
    let order: Int
    let targetSets: Int
    var targetRepsMin: Int?
    var targetRepsMax: Int?
    var targetRir: Int?

    var lastDate: String?
    var lastWeightKg: Double?
    var lastReps: Int?
    var lastRir: Int?

    /// 今日すでに記録したか
    var doneToday: Bool?

    /// 「前回 80×8 RIR2」。**未実施は「—」**（空文字だと行の高さが揃わない）
    var lastLabel: String {
        guard let w = lastWeightKg, let r = lastReps else { return "—" }

        let base = "前回 \(numberText(w))×\(r)"

        return lastRir.map { "\(base) RIR\($0)" } ?? base
    }

    /// 「5セット 6-10」。レップ指定が無ければセット数だけ
    var targetLabel: String {
        let sets = "\(targetSets)セット"
        switch (targetRepsMin, targetRepsMax) {
        case let (min?, max?): return "\(sets) \(min)-\(max)"
        case let (min?, nil): return "\(sets) \(min)-"
        case let (nil, max?): return "\(sets) -\(max)"
        default: return sets
        }
    }
}
