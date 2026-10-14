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
