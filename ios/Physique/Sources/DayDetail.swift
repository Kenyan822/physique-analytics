import Foundation

/// その日1日ぶんの要約（#276）。
///
/// **継続カレンダーから降りるための画面。** 「10/3 が未達なのはなぜか」を
/// 見るのに食事タブへ移動して日付を合わせ直すのでは、カレンダーの意味が無い。
struct DayDetail: Codable, Sendable {
    let date: String
    let meals: DayMeals
    var workout: DayWorkout?
    var body: DayBody?
}

struct DayMeals: Codable, Sendable {
    let consumed: Macros
    /// その日に有効だった目標。**引けなければ nil**
    var target: Macros?
    /// P・F・C すべてが ±10% 以内か。**判定できない日は nil**（#248 と同じ定義）
    var goalMet: Bool?
    /// 目標との差。マイナスが不足。
    /// **kcal は持たない** —— PFC から導けるので、サーバも返さない
    var shortfall: MacroDiff?

    /// 未達の理由。**足りない順**に並べる —— 一番効いているものから直したい。
    ///
    /// 達成した日と、判定できない日は空。
    /// **「未達」と「判定できない」を混ぜない**（#248 と同じ）
    var reasons: [String] {
        guard goalMet == false, let s = shortfall else { return [] }

        let items: [(String, Double)] = [
            ("P", s.proteinG), ("F", s.fatG), ("C", s.carbG),
        ]

        return items
            .filter { $0.1 < 0 }
            .sorted { abs($0.1) > abs($1.1) }
            .map { "\($0.0) が \(numberText(abs($0.1)))g 足りない" }
    }
}

/// 目標との差（`MacroDiff`）。**kcal は無い**
struct MacroDiff: Codable, Sendable {
    let proteinG: Double
    let fatG: Double
    let carbG: Double
}

struct DayWorkout: Codable, Sendable {
    /// その日のセッションのテンプレート名。**ルーティン外でやった日は nil**
    var templateName: String?
    var dayOrder: Int?
    let setCount: Int
    var exercises: [DayWorkoutExercise]?
}

struct DayWorkoutExercise: Codable, Sendable, Identifiable {
    var id: String { exerciseName }

    let exerciseName: String
    let setCount: Int
    var topWeightKg: Double?
}

struct DayBody: Codable, Sendable {
    var weightKg: Double?
    var bodyFatPct: Double?
}
