import Charts
import SwiftUI

/// 折れ線1つぶん。**体組成と食事で同じものを使う**（#307 / #308）。
///
/// 数字を並べるだけ。判定も提案もしない（ADR-0010）。
struct TrendChart: View {
    let points: [TrendPoint]
    let unit: String
    let isLoading: Bool
    /// 記録が無いときに出す一文
    let emptyNote: String

    var body: some View {
        if points.isEmpty {
            // **空のグラフを描かない。** 軸だけ出ても何も分からない
            ContentUnavailableView(
                isLoading ? "読み込み中" : "記録が無い",
                systemImage: "chart.xyaxis.line",
                description: Text(isLoading ? "" : emptyNote)
            )
            .frame(height: 300)
        } else {
            Chart(points) { p in
                LineMark(x: .value("日", p.date), y: .value(unit, p.value))
                    .interpolationMethod(.monotone)

                PointMark(x: .value("日", p.date), y: .value(unit, p.value))
                    .symbolSize(16)
            }
            // **0 を含めない。** 体重 70〜72kg を 0 から描くと真っ平らに見える
            .chartYScale(domain: .automatic(includesZero: false))
            .chartYAxisLabel(unit)
            .frame(height: 300)
            .accessibilityIdentifier("trendChart")
        }
    }
}
