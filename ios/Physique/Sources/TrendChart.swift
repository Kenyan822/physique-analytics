import Charts
import SwiftUI

/// 折れ線1つぶん。**体組成と食事で同じものを使う**（#307 / #308 / #318）。
///
/// 数字を並べるだけ。判定も提案もしない（ADR-0010）。
struct TrendChart: View {
    let points: [TrendPoint]
    let unit: String
    let span: TrendSpan
    let isLoading: Bool
    /// 記録が無いときに出す一文
    let emptyNote: String

    /// タップした位置。**点と一致しない**ので、一番近い点に寄せる
    @State private var tappedAt: Date?

    private var selected: TrendPoint? {
        guard let tappedAt else { return nil }

        return TrendSeries.nearest(points, to: tappedAt)
    }

    var body: some View {
        if points.isEmpty {
            // **空のグラフを描かない。** 軸だけ出ても何も分からない
            ContentUnavailableView(
                isLoading ? "読み込み中" : "記録が無い",
                systemImage: "chart.xyaxis.line",
                description: Text(isLoading ? "" : emptyNote)
            )
            .frame(height: 340)
        } else {
            VStack(alignment: .leading, spacing: 4) {
                readout
                chart
            }
        }
    }

    /// 選んだ点の数値。**選んでいなければ一番新しい点**を出す。
    /// 空にしておくと、タップするまで数字が1つも読めない。
    private var readout: some View {
        let p = selected ?? points.last

        return HStack(alignment: .firstTextBaseline, spacing: 6) {
            if let p {
                Text(span.pointLabel(p.date))
                    .font(.subheadline).foregroundStyle(.secondary)
                Text(bodyText(p.value))
                    .font(.title2).bold().monospacedDigit()
                    .accessibilityIdentifier("trendValue")
                Text(unit).font(.caption).foregroundStyle(.secondary)
            }
            Spacer()
            if selected == nil {
                Text("最新").font(.caption2).foregroundStyle(.tertiary)
            }
        }
        .frame(height: 30)
    }

    private var chart: some View {
        Chart(points) { p in
            LineMark(x: .value("日", p.date), y: .value(unit, p.value))
                .interpolationMethod(.monotone)

            PointMark(x: .value("日", p.date), y: .value(unit, p.value))
                .symbolSize(selected?.id == p.id ? 120 : 16)

            if selected?.id == p.id {
                RuleMark(x: .value("日", p.date))
                    .foregroundStyle(.secondary.opacity(0.3))
                    .lineStyle(StrokeStyle(lineWidth: 1, dash: [3, 3]))
            }
        }
        // **0 を含めない。** 体重 70〜72kg を 0 から描くと真っ平らに見える
        .chartYScale(domain: .automatic(includesZero: false))
        .chartYAxisLabel(unit)
        .chartXAxis {
            // **自動整形に任せない。** 幅が足りないと月だけになり、
            // 2年ぶん並べると「どの6月か」が分からなくなる
            AxisMarks(values: .automatic(desiredCount: 5)) { value in
                AxisGridLine()
                AxisTick()
                AxisValueLabel {
                    if let d = value.as(Date.self) {
                        Text(span.axisLabel(d))
                    }
                }
            }
        }
        .chartXSelection(value: $tappedAt)
        .frame(height: 300)
        .accessibilityIdentifier("trendChart")
    }
}
