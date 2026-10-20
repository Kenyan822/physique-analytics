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

/// 種目を登録するときに送る内容（#264）。
struct ExerciseInput: Codable, Sendable {
    /// クライアント生成の UUID。再送が冪等になる
    var id: UUID?
    var name: String
    var muscleGroup: MuscleGroup
    var isCompound: Bool?
    /// インターバルの既定。未指定なら多関節かどうかで決まる
    var defaultRestSec: Int?
}

struct WorkoutSet: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let sessionId: UUID
    let exerciseId: UUID
    let setNo: Int
    let weightKg: Double
    /// 自重の種目か（#335）。**立っているときの挙上重量は
    /// 「その日の体重 + weightKg」。** weightKg は加重のぶん
    var bodyweight = false
    let reps: Int
    var rir: Int?

    // **合成の Codable は既定値を使わない。** キーが無いと decode が投げ、
    // セットが1件も読めなくなる。足したばかりの項目は必ず decodeIfPresent で読む
    init(from decoder: Decoder) throws {
        let c = try decoder.container(keyedBy: CodingKeys.self)
        id = try c.decode(UUID.self, forKey: .id)
        sessionId = try c.decode(UUID.self, forKey: .sessionId)
        exerciseId = try c.decode(UUID.self, forKey: .exerciseId)
        setNo = try c.decode(Int.self, forKey: .setNo)
        weightKg = try c.decode(Double.self, forKey: .weightKg)
        bodyweight = try c.decodeIfPresent(Bool.self, forKey: .bodyweight) ?? false
        reps = try c.decode(Int.self, forKey: .reps)
        rir = try c.decodeIfPresent(Int.self, forKey: .rir)
    }

    init(id: UUID, sessionId: UUID, exerciseId: UUID, setNo: Int,
         weightKg: Double, bodyweight: Bool = false, reps: Int, rir: Int? = nil) {
        self.id = id
        self.sessionId = sessionId
        self.exerciseId = exerciseId
        self.setNo = setNo
        self.weightKg = weightKg
        self.bodyweight = bodyweight
        self.reps = reps
        self.rir = rir
    }
}

struct WorkoutSession: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    /// JST における日付（ADR-0013）。`yyyy-MM-dd`
    let date: String
    var note: String?
    var sets: [WorkoutSet]
    /// その日の種目リスト。**空 = まだ触っていない**のでルーティンに従う（#242）
    var exercises: [SessionExercise]?
}

/// その日の種目リストの1行（#242）。
struct SessionExercise: Codable, Identifiable, Hashable, Sendable {
    var id: UUID { exerciseId }

    let exerciseId: UUID
    let exerciseName: String
    let muscleGroup: MuscleGroup
    let itemOrder: Int
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
    var bodyweight: Bool = false
    var reps: Int
    var rir: Int?
}

struct WorkoutSessionInput: Codable, Sendable {
    var id: UUID?
    var date: String
    /// どの Day をやったか（要件 T-01 / #232）。
    ///
    /// **これを入れないと巡回が進まない。** 次に何日目かは
    /// 直近のセッションの template_id から導いている
    var templateId: UUID?
    var note: String?
    var sets: [WorkoutSetInput]?
}
