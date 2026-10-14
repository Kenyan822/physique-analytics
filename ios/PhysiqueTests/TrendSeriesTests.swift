import Foundation
import Testing

@testable import PhysiqueCore

/// 推移グラフに渡す点の作り方（要件 A-06 / #307）。
///
/// **ここで見るのは畳み方だけ。** 描画は実機で見る。
@Suite("推移グラフの点")
struct TrendSeriesTests {
    /// 実データを使わないための器（ADR-0002）
    private func daily(_ date: String, weight: Double?, bodyfat: Double?) -> DailyMetrics {
        DailyMetrics(id: UUID(), date: date, weightKg: weight, bodyfatPct: bodyfat, fatigue: nil)
    }

    @Test("値が無い日は点にしない")
    func skipsMissing() {
        // **0 として点を打つと線が床まで落ちる。** 測っていないだけ
        let items = [
            daily("2028-06-01", weight: 70, bodyfat: 20),
            daily("2028-06-02", weight: nil, bodyfat: 20),
            daily("2028-06-03", weight: 71, bodyfat: nil),
        ]

        let got = TrendSeries.points(items, .weight, span: .daily)
        #expect(got.count == 2)
        #expect(got.map(\.label) == ["2028-06-01", "2028-06-03"])
    }

    @Test("日付順に並ぶ")
    func sorted() {
        let items = [
            daily("2028-06-03", weight: 71, bodyfat: nil),
            daily("2028-06-01", weight: 70, bodyfat: nil),
            daily("2028-06-02", weight: 72, bodyfat: nil),
        ]

        #expect(TrendSeries.points(items, .weight, span: .daily).map(\.label)
            == ["2028-06-01", "2028-06-02", "2028-06-03"])
    }

    @Test("月ごとは平均になる")
    func monthlyAverage() {
        let items = [
            daily("2028-06-01", weight: 70, bodyfat: nil),
            daily("2028-06-15", weight: 72, bodyfat: nil),
            daily("2028-07-01", weight: 68, bodyfat: nil),
        ]

        let got = TrendSeries.points(items, .weight, span: .monthly)
        #expect(got.map(\.label) == ["2028-06", "2028-07"])
        #expect(abs(got[0].value - 71) < 0.001)
        #expect(abs(got[1].value - 68) < 0.001)
    }

    @Test("月の平均は値がある日だけで割る")
    func monthlyIgnoresMissing() {
        // **測っていない日を 0 で数えると平均が下振れする**
        let items = [
            daily("2028-06-01", weight: 70, bodyfat: nil),
            daily("2028-06-02", weight: nil, bodyfat: nil),
            daily("2028-06-03", weight: 72, bodyfat: nil),
        ]

        let got = TrendSeries.points(items, .weight, span: .monthly)
        #expect(got.count == 1)
        #expect(abs(got[0].value - 71) < 0.001)
    }

    @Test("除脂肪体重は両方揃った日だけ")
    func leanNeedsBoth() {
        let items = [
            daily("2028-06-01", weight: 80, bodyfat: 20),
            daily("2028-06-02", weight: 80, bodyfat: nil),
            daily("2028-06-03", weight: nil, bodyfat: 20),
        ]

        let got = TrendSeries.points(items, .lean, span: .daily)
        #expect(got.count == 1)
        #expect(abs(got[0].value - 64) < 0.001)
    }

    @Test("記録が無ければ空")
    func empty() {
        // 空のグラフを描かない。呼び出し側が「記録が無い」と出せるようにする
        #expect(TrendSeries.points([], .weight, span: .daily).isEmpty)
        #expect(TrendSeries.points([], .weight, span: .monthly).isEmpty)
    }

    @Test("月の点は月初に置く")
    func monthlyDateIsFirstOfMonth() {
        let items = [daily("2028-06-15", weight: 70, bodyfat: nil)]

        let got = TrendSeries.points(items, .weight, span: .monthly)
        #expect(got.count == 1)
        #expect(JST.dateString(from: got[0].date) == "2028-06-01")
    }

    @Test("読めない日付は落とす")
    func dropsBadDates() {
        // **壊れた点を打たない。** サーバが想定外の形を返しても画面を壊さない
        let items = [
            daily("2028-06-01", weight: 70, bodyfat: nil),
            daily("2028/06/02", weight: 71, bodyfat: nil),
        ]

        #expect(TrendSeries.points(items, .weight, span: .daily).count == 1)
    }
}

/// 食事の推移（要件 N-05 / #308）。
///
/// **日ごとに合計してから畳む。** 1日に何件も記録されるので、
/// 体組成と同じようには扱えない。
@Suite("食事の推移グラフの点")
struct MealTrendSeriesTests {
    private func meal(_ date: String, kcal: Int?, p: Double? = nil) -> Meal {
        Meal(id: UUID(), date: date, at: nil, slot: nil, name: nil, qty: nil,
             kcal: kcal, proteinG: p, fatG: nil, carbG: nil, source: nil)
    }

    @Test("同じ日の食事を合計する")
    func sumsPerDay() {
        let meals = [
            meal("2028-06-01", kcal: 500),
            meal("2028-06-01", kcal: 700),
            meal("2028-06-02", kcal: 600),
        ]

        let got = TrendSeries.points(meals, .kcal, span: .daily)
        #expect(got.map(\.label) == ["2028-06-01", "2028-06-02"])
        #expect(abs(got[0].value - 1200) < 0.001)
        #expect(abs(got[1].value - 600) < 0.001)
    }

    @Test("値が無い記録は合計に入れない")
    func skipsMissingValues() {
        // 名前だけの記録がある（#188）。**0 として足すと合計が変わらないが、
        // 「記録がある日」に見えてしまう**ので、値のある件だけ数える
        let meals = [
            meal("2028-06-01", kcal: 500),
            meal("2028-06-01", kcal: nil),
        ]

        let got = TrendSeries.points(meals, .kcal, span: .daily)
        #expect(got.count == 1)
        #expect(abs(got[0].value - 500) < 0.001)
    }

    @Test("その日に1件も値が無ければ点にしない")
    func noPointWithoutValues() {
        // **0 kcal の日ではなく「記録していない日」。** 線を床まで落とさない
        let meals = [meal("2028-06-01", kcal: nil), meal("2028-06-01", kcal: nil)]

        #expect(TrendSeries.points(meals, .kcal, span: .daily).isEmpty)
    }

    @Test("月ごとは日ごとの合計の平均")
    func monthlyAveragesDailyTotals() {
        // 1日目 1200、2日目 600 → 平均 900。**合計の 1800 ではない**
        let meals = [
            meal("2028-06-01", kcal: 500),
            meal("2028-06-01", kcal: 700),
            meal("2028-06-02", kcal: 600),
        ]

        let got = TrendSeries.points(meals, .kcal, span: .monthly)
        #expect(got.map(\.label) == ["2028-06"])
        #expect(abs(got[0].value - 900) < 0.001)
    }

    @Test("記録が無い日は月の平均に入れない")
    func monthlyIgnoresUnrecordedDays() {
        // 1日しか記録が無い月の平均は、その日の値そのもの
        let meals = [meal("2028-06-15", kcal: 2000)]

        let got = TrendSeries.points(meals, .kcal, span: .monthly)
        #expect(got.count == 1)
        #expect(abs(got[0].value - 2000) < 0.001)
    }

    @Test("タンパク質も同じように出せる")
    func protein() {
        let meals = [
            meal("2028-06-01", kcal: 500, p: 30),
            meal("2028-06-01", kcal: 700, p: 20),
        ]

        let got = TrendSeries.points(meals, .protein, span: .daily)
        #expect(abs(got[0].value - 50) < 0.001)
    }

    @Test("読めない日付は落とす")
    func dropsBadDates() {
        let meals = [meal("2028-06-01", kcal: 500), meal("2028/06/02", kcal: 600)]

        #expect(TrendSeries.points(meals, .kcal, span: .daily).count == 1)
    }

    @Test("記録が無ければ空")
    func empty() {
        #expect(TrendSeries.points([Meal](), .kcal, span: .daily).isEmpty)
        #expect(TrendSeries.points([Meal](), .kcal, span: .monthly).isEmpty)
    }
}

/// 横軸の表示とタップした点の見つけ方（#318）。
@Suite("推移グラフの読みやすさ")
struct TrendReadabilityTests {
    private func point(_ day: String, _ v: Double) -> TrendPoint {
        TrendPoint(label: day, date: JST.date(from: day) ?? Date(), value: v)
    }

    @Test("横軸は日ごとなら月日まで出す")
    func axisDaily() {
        // **既定の自動整形だと月しか出ないことがある。** 日まで出す
        #expect(TrendSpan.daily.axisLabel(JST.date(from: "2028-06-15")!) == "6/15")
    }

    @Test("横軸は月ごとなら年月を出す")
    func axisMonthly() {
        // 2年ぶん並ぶので、年が無いとどの 6月か分からない
        #expect(TrendSpan.monthly.axisLabel(JST.date(from: "2028-06-01")!) == "2028/6")
    }

    @Test("選んだ点は曜日まで出す")
    func pointLabelDaily() {
        // **曜日が要る。** トレ日かどうかが日付だけでは分からない
        #expect(TrendSpan.daily.pointLabel(JST.date(from: "2028-06-15")!) == "2028/6/15(木)")
    }

    @Test("選んだ点は月ごとなら年月")
    func pointLabelMonthly() {
        #expect(TrendSpan.monthly.pointLabel(JST.date(from: "2028-06-01")!) == "2028年6月")
    }

    @Test("タップに一番近い点を返す")
    func nearest() {
        // **指の位置は点と一致しない。** 一番近い点に寄せる
        let points = [point("2028-06-01", 70), point("2028-06-10", 71), point("2028-06-20", 72)]

        let got = TrendSeries.nearest(points, to: JST.date(from: "2028-06-12")!)
        #expect(got?.label == "2028-06-10")
    }

    @Test("端より外でも一番近い点を返す")
    func nearestOutside() {
        let points = [point("2028-06-01", 70), point("2028-06-10", 71)]

        #expect(TrendSeries.nearest(points, to: JST.date(from: "2028-05-01")!)?.label == "2028-06-01")
        #expect(TrendSeries.nearest(points, to: JST.date(from: "2028-12-01")!)?.label == "2028-06-10")
    }

    @Test("点が無ければ nil")
    func nearestEmpty() {
        #expect(TrendSeries.nearest([], to: Date()) == nil)
    }
}
