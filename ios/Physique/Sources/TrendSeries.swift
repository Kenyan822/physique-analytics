import Foundation

/// 推移グラフに出す指標（要件 A-06）。
///
/// **ここで扱うのは「数値の推移を見る」だけ。** 判定も提案もしない。
/// 事前に定義できない分析は MCP の担当（[ADR-0010](../../../docs/adr/0010-mcp-over-analysis-ui.md)）。
enum BodyMetric: String, CaseIterable, Identifiable, Sendable {
    case weight = "体重"
    case bodyfat = "体脂肪率"
    case lean = "除脂肪体重"

    var id: String { rawValue }

    var unit: String {
        switch self {
        case .weight, .lean: "kg"
        case .bodyfat: "%"
        }
    }

    /// その日の値。**測っていなければ nil。**
    func value(in d: DailyMetrics) -> Double? {
        switch self {
        case .weight: d.weightKg
        case .bodyfat: d.bodyfatPct
        case .lean: leanBodyMass(weightKg: d.weightKg, bodyfatPct: d.bodyfatPct)
        }
    }
}

/// 食事の推移に出す指標（要件 N-05 / A-09）。
///
/// **daily_metrics の kcal 列は使わない。** 書かれていないので、
/// 食事の記録から日ごとに合計する。
enum MealMetric: String, CaseIterable, Identifiable, Sendable {
    case kcal = "kcal"
    case protein = "P"
    case fat = "F"
    case carb = "C"

    var id: String { rawValue }

    var unit: String {
        switch self {
        case .kcal: "kcal"
        case .protein, .fat, .carb: "g"
        }
    }

    /// その1件の値。**記録されていなければ nil。**
    func value(in m: Meal) -> Double? {
        switch self {
        case .kcal: m.kcal.map(Double.init)
        case .protein: m.proteinG
        case .fat: m.fatG
        case .carb: m.carbG
        }
    }
}

/// 日ごとに見るか、月ごとに見るか。
enum TrendSpan: String, CaseIterable, Identifiable, Sendable {
    case daily = "日"
    case monthly = "月"

    var id: String { rawValue }

    /// さかのぼる日数。
    ///
    /// 日ごとに2年ぶん出しても潰れて読めないので、粒度で変える。
    var days: Int {
        switch self {
        case .daily: 90
        case .monthly: 730
        }
    }
}

/// グラフの1点。
struct TrendPoint: Identifiable, Hashable, Sendable {
    /// 日ごとは `yyyy-MM-dd`、月ごとは `yyyy-MM`
    let label: String
    /// 横軸に使う日。月ごとは月初（JST）
    let date: Date
    let value: Double

    var id: String { label }
}

/// `yyyy-MM-dd` を読む。読めなければ nil。
///
/// **往復で確かめる。** `DateFormatter` は区切り文字に寛容で、
/// `2028/06/02` も通してしまう。整形し直して一致を見るのが確実。
private func parseDay(_ s: String) -> Date? {
    guard let d = JST.date(from: s), JST.dateString(from: d) == s else { return nil }

    return d
}

/// 日次記録を推移グラフの点に畳む。
enum TrendSeries {
    static func points(
        _ items: [DailyMetrics], _ metric: BodyMetric, span: TrendSpan
    ) -> [TrendPoint] {
        switch span {
        case .daily: dailyPoints(items, metric)
        case .monthly: monthlyPoints(items, metric)
        }
    }

    /// **値が無い日は点にしない。** 0 を打つと線が床まで落ちて、
    /// 「測っていない」が「0だった」に見える。
    private static func dailyPoints(_ items: [DailyMetrics], _ metric: BodyMetric) -> [TrendPoint] {
        items
            .compactMap { d -> TrendPoint? in
                guard let v = metric.value(in: d), let date = parseDay(d.date) else {
                    return nil
                }

                return TrendPoint(label: d.date, date: date, value: v)
            }
            .sorted { $0.label < $1.label }
    }

    /// 月ごとの平均。**値がある日だけで割る。**
    /// 測っていない日を 0 で数えると平均が下振れする。
    private static func monthlyPoints(_ items: [DailyMetrics], _ metric: BodyMetric) -> [TrendPoint] {
        var byMonth: [String: (total: Double, count: Int)] = [:]

        for d in items {
            // 読めない日付は落とす。壊れた点を打たない
            guard let v = metric.value(in: d), parseDay(d.date) != nil else { continue }

            let month = String(d.date.prefix(7))
            let cur = byMonth[month] ?? (0, 0)
            byMonth[month] = (cur.total + v, cur.count + 1)
        }

        return byMonth.keys.sorted().compactMap { month in
            guard let s = byMonth[month], s.count > 0,
                  let date = parseDay(month + "-01")
            else { return nil }

            return TrendPoint(label: month, date: date, value: s.total / Double(s.count))
        }
    }
}

extension TrendSeries {
    /// 食事を日ごとに合計してから畳む。
    ///
    /// 体組成と違って**1日に何件も記録される**ので、先に合計する。
    ///
    /// **その日に1件も値が無ければ点を作らない。** 名前だけの記録しか
    /// 無い日は「0 kcal の日」ではなく「測っていない日」なので、
    /// 線を床まで落とさない。
    static func points(
        _ meals: [Meal], _ metric: MealMetric, span: TrendSpan
    ) -> [TrendPoint] {
        let totals = dailyTotals(meals, metric)

        switch span {
        case .daily:
            return totals.keys.sorted().compactMap { day in
                guard let v = totals[day], let date = parseDay(day) else { return nil }

                return TrendPoint(label: day, date: date, value: v)
            }

        case .monthly:
            var byMonth: [String: (total: Double, days: Int)] = [:]
            for (day, v) in totals {
                let month = String(day.prefix(7))
                let cur = byMonth[month] ?? (0, 0)
                byMonth[month] = (cur.total + v, cur.days + 1)
            }

            // **日ごとの合計の平均**（月の合計ではない）。1日あたりの摂取を見る
            return byMonth.keys.sorted().compactMap { month in
                guard let s = byMonth[month], s.days > 0,
                      let date = parseDay(month + "-01")
                else { return nil }

                return TrendPoint(label: month, date: date, value: s.total / Double(s.days))
            }
        }
    }

    /// 日ごとの合計。値が1件も無い日は作らない。
    private static func dailyTotals(_ meals: [Meal], _ metric: MealMetric) -> [String: Double] {
        var totals: [String: Double] = [:]

        for m in meals {
            // 読めない日付は落とす。壊れた点を打たない
            guard let v = metric.value(in: m), parseDay(m.date) != nil else { continue }
            totals[m.date, default: 0] += v
        }

        return totals
    }
}
