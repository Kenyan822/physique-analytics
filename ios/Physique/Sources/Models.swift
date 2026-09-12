import Foundation

/// 部位。`openapi.yaml` の MuscleGroup と一対一で対応する。
///
/// 肩を前部/中部/後部、背中を広背筋/僧帽筋に分けているのは、
/// 一括だと部位内の偏り（プレス系で前部だけ伸びる等）が埋もれるため。
enum MuscleGroup: String, Codable, CaseIterable, Sendable {
    case chest = "胸"
    case lats = "広背筋"
    case traps = "僧帽筋"
    case frontDelts = "肩前部"
    case sideDelts = "肩中部"
    case rearDelts = "肩後部"
    case biceps = "上腕二頭"
    case triceps = "上腕三頭"
    case quads = "大腿四頭"
    case hamstrings = "ハム"
    case glutes = "臀部"
    case calves = "ふくらはぎ"
    case abs = "腹"
}

struct Exercise: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let name: String
    let muscleGroup: MuscleGroup
    var isCompound: Bool?
    var defaultRestSec: Int?
}

struct WorkoutSet: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let sessionId: UUID
    let exerciseId: UUID
    let setNo: Int
    let weightKg: Double
    let reps: Int
    var rir: Int?
}

struct WorkoutSession: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    /// JST における日付（ADR-0013）。`yyyy-MM-dd`
    let date: String
    var note: String?
    var sets: [WorkoutSet]
}

/// 前回の実施内容（要件 T-02）。入力速度を決める最重要データ。
struct LastPerformance: Codable, Sendable {
    let exerciseId: UUID
    /// 未実施なら nil
    var date: String?
    var sets: [WorkoutSet]?
    var estimatedOneRm: Double?
}

/// RFC 7807。API のエラーはこの形で統一されている。
struct Problem: Codable, Sendable {
    let type: String
    let title: String
    let status: Int
    var detail: String?
}

// MARK: - 入力

struct WorkoutSetInput: Codable, Sendable {
    /// クライアントが生成する UUID。再送の冪等性に使う（ADR-0014）
    var id: UUID?
    var exerciseId: UUID
    var setNo: Int
    var weightKg: Double
    var reps: Int
    var rir: Int?
}

struct WorkoutSessionInput: Codable, Sendable {
    var id: UUID?
    var date: String
    var note: String?
    var sets: [WorkoutSetInput]?
}
