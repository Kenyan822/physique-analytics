import Foundation

/// 食事の区分。`openapi.yaml` の MealSlot と一対一で対応する。
enum MealSlot: String, Codable, CaseIterable, Sendable, Identifiable {
    case breakfast = "朝食"
    case lunch = "昼食"
    case dinner = "夕食"
    case snack = "間食"

    var id: String { rawValue }

    /// 時刻から推測する。
    ///
    /// **開くたびに選び直させない。** 外れていても1タップで直せる方が、
    /// 毎回4択から選ぶより速い。境界は食事の時間帯としてありがちな値で切った。
    static func suggested(at date: Date = Date(), calendar: Calendar = JST.calendar) -> MealSlot {
        switch calendar.component(.hour, from: date) {
        case 5..<11: return .breakfast
        case 11..<16: return .lunch
        case 16..<22: return .dinner
        default: return .snack
        }
    }
}

/// 手入力か AI 推定か。推定値の比率が高い週は分析の確度を下げて扱う
enum MealSource: String, Codable, Sendable {
    case manual
    case aiEstimated = "ai_estimated"
}

struct Meal: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let date: String
    /// 食べた時刻 "HH:MM"（JST）。**過去の記録には無い**ので optional
    var at: String?
    var slot: MealSlot?
    /// **任意。** PFC だけの記録を許す（#188）
    var name: String?
    var qty: String?
    var kcal: Int?
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?
    var source: MealSource?
}

/// 記録するときに送る内容。
struct MealInput: Codable, Sendable {
    /// クライアントで振る。再送が冪等になる
    var id: UUID?
    var date: String
    /// 食べた時刻 "HH:MM"（JST）。**送ればサーバが区分を導出する**ので、
    /// 区分を選ばせる必要が無い
    var at: String?
    var slot: MealSlot?
    var name: String?
    var qty: String?
    var kcal: Int?
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?
    var source: MealSource?
}

/// 過去の記録から作る候補（要件 N-02）。
///
/// **食品マスタを持たない。** 記録がそのままマスタになる
struct MealSuggestion: Codable, Identifiable, Hashable, Sendable {
    var id: String { name }
    let name: String
    let count: Int
    var lastDate: String?
    var qty: String?
    var kcal: Int?
    var proteinG: Double?
    var fatG: Double?
    var carbG: Double?
}

struct Macros: Codable, Hashable, Sendable {
    let kcal: Int
    let proteinG: Double
    let fatG: Double
    let carbG: Double
}

/// その日の摂取目標（要件 N-05）。
struct DailyTargets: Codable, Sendable {
    let date: String
    var phase: String?
    var target: Macros?
    var consumed: Macros?
    var note: String?
}

/// その日の合計。
struct MealTotals: Sendable, Equatable {
    var kcal: Int
    var proteinG: Double
    var fatG: Double
    var carbG: Double
    /// PFC が入っていない記録の数。**0 として足さない** ——
    /// 0 と混ぜると「食べたが記録が雑」と「食べていない」が同じに見える
    var withoutMacros: Int

    init(kcal: Int, proteinG: Double, fatG: Double, carbG: Double, withoutMacros: Int) {
        self.kcal = kcal
        self.proteinG = proteinG
        self.fatG = fatG
        self.carbG = carbG
        self.withoutMacros = withoutMacros
    }

    init(of meals: [Meal]) {
        kcal = meals.compactMap(\.kcal).reduce(0, +)
        proteinG = meals.compactMap(\.proteinG).reduce(0, +)
        fatG = meals.compactMap(\.fatG).reduce(0, +)
        carbG = meals.compactMap(\.carbG).reduce(0, +)
        withoutMacros = meals.filter {
            $0.kcal == nil && $0.proteinG == nil && $0.fatG == nil && $0.carbG == nil
        }.count
    }

    /// 目標に対する残り。**超えたら負のまま返す** ——
    /// 0 で止めると「あとどれだけ削るか」が分からない
    func remaining(from target: Macros) -> Macros {
        Macros(
            kcal: target.kcal - kcal,
            proteinG: target.proteinG - proteinG,
            fatG: target.fatG - fatG,
            carbG: target.carbG - carbG
        )
    }
}
