import Foundation
import Observation

/// 日別の達成（#248 / #249）。
struct StreakDay: Codable, Sendable, Identifiable {
    var id: String { date }

    let date: String
    /// P・F・C すべてが当日の目標の ±10% 以内。**目標が引けない日は nil**
    let mealGoalMet: Bool?
    let trained: Bool
}

/// 月カレンダーの状態（要件 A-xx / #249）。
///
/// **続いているかを一目で見るための画面。** 細かい数字は食事・記録の各画面にある。
@Observable
@MainActor
final class StreakModel {
    private(set) var days: [String: StreakDay] = [:]
    private(set) var loading = false
    private(set) var errorMessage = ""
    var showError = false

    /// 表示している月の1日。`yyyy-MM-01`
    private(set) var month: String
    /// 「今日」。これより先の月には進めない。テストで固定するため引数にする
    private let today: String
    private let api: APIClient

    init(api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL), today: String = JST.dateString()) {
        self.api = api
        self.today = today
        month = String(today.prefix(7)) + "-01"
    }

    // MARK: - 表示

    var monthLabel: String {
        let p = month.split(separator: "-")
        guard p.count >= 2, let y = Int(p[0]), let m = Int(p[1]) else { return month }

        return "\(y)年\(m)月"
    }

    /// 月の日数
    var daysInMonth: Int {
        guard let d = JST.date(from: month),
              let r = JST.calendar.range(of: .day, in: .month, for: d) else { return 30 }

        return r.count
    }

    /// 日曜始まりの升目で、1日の前に空ける数
    var leadingBlanks: Int {
        guard let d = JST.date(from: month) else { return 0 }

        return JST.calendar.component(.weekday, from: d) - 1
    }

    /// **今日より先の月には進めない。** 記録のしようがない月を見ても意味が無い
    var canGoNext: Bool { month < String(today.prefix(7)) + "-01" }

    func day(_ date: String) -> StreakDay? { days[date] }

    /// その月の n 日目の日付文字列
    func date(ofDay n: Int) -> String {
        String(month.prefix(8)) + String(format: "%02d", n)
    }

    // MARK: - 連続日数

    /// 食事の目標を満たした連続日数。**今日から遡る。**
    ///
    /// **判定できない日（nil）では切らない。** 目標を決める前の日や、
    /// 自動計算もできない日が挟まっただけで途切れるのは実態に合わない
    var mealStreak: Int { streak { $0.mealGoalMet } }

    /// 筋トレの連続日数。**休養日で切れる**ので参考値
    var trainedStreak: Int { streak { $0.trained } }

    private func streak(_ met: (StreakDay) -> Bool?) -> Int {
        var n = 0
        var cursor = today

        while let d = days[cursor] {
            switch met(d) {
            case true: n += 1
            case false: return n
            case nil: break    // 判定できない日は飛ばす
            }
            cursor = JST.shift(cursor, days: -1)
        }

        return n
    }

    // MARK: - 読み込み

    func load() async {
        loading = true
        defer { loading = false }

        do {
            let items = try await api.streaks(from: month, to: date(ofDay: daysInMonth))
            days = Dictionary(uniqueKeysWithValues: items.map { ($0.date, $0) })
        } catch {
            errorMessage = (error as? APIError)?.errorDescription ?? error.localizedDescription
            showError = true
        }
    }

    func goToPreviousMonth() async {
        await move(by: -1)
    }

    func goToNextMonth() async {
        guard canGoNext else { return }
        await move(by: 1)
    }

    private func move(by months: Int) async {
        guard let d = JST.date(from: month),
              let moved = JST.calendar.date(byAdding: .month, value: months, to: d) else { return }
        month = String(JST.dateString(from: moved).prefix(8)) + "01"
        await load()
    }
}
