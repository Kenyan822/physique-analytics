import Charts
import SwiftUI

/// 体組成の推移（要件 A-06 / #307）。
///
/// **数値の推移を見るだけの画面。** 判定も提案もしない。
/// 事前に定義できない分析は MCP の担当（ADR-0010）。
struct BodyChartView: View {
    @State private var model: BodyChartModel

    init(api: APIClient, today: String = JST.dateString()) {
        _model = State(initialValue: BodyChartModel(api: api, today: today))
    }

    var body: some View {
        VStack(spacing: 12) {
            Picker("指標", selection: $model.metric) {
                ForEach(BodyMetric.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("metricPicker")

            Picker("粒度", selection: $model.span) {
                ForEach(TrendSpan.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
            .frame(maxWidth: 180)
            .accessibilityIdentifier("spanPicker")

            chart

            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(.secondary)
            }

            Spacer()
        }
        .padding()
        .navigationTitle("推移")
        .navigationBarTitleDisplayMode(.inline)
        // 粒度を変えると取る期間が変わるので読み直す。指標は同じデータで切り替わる
        .task(id: model.span) { await model.load() }
    }

    @ViewBuilder
    private var chart: some View {
        let points = model.points

        if points.isEmpty {
            // **空のグラフを描かない。** 軸だけ出ても何も分からない
            ContentUnavailableView(
                model.isLoading ? "読み込み中" : "記録が無い",
                systemImage: "chart.xyaxis.line",
                description: Text(model.isLoading ? "" : "\(model.span.days)日さかのぼって記録が見つからない")
            )
            .frame(height: 300)
        } else {
            Chart(points) { p in
                LineMark(
                    x: .value("日", p.date),
                    y: .value(model.metric.rawValue, p.value)
                )
                .interpolationMethod(.monotone)

                PointMark(
                    x: .value("日", p.date),
                    y: .value(model.metric.rawValue, p.value)
                )
                .symbolSize(16)
            }
            // **0 を含めない。** 体重 70〜72kg を 0 から描くと真っ平らに見える
            .chartYScale(domain: .automatic(includesZero: false))
            .chartYAxisLabel(model.metric.unit)
            .frame(height: 300)
            .accessibilityIdentifier("trendChart")
        }
    }
}

/// 推移グラフの状態。
@Observable
@MainActor
final class BodyChartModel {
    var metric: BodyMetric = .weight
    var span: TrendSpan = .daily

    private(set) var isLoading = false
    private(set) var message: String?

    private var items: [DailyMetrics] = []
    private let api: APIClient
    private let today: String

    init(api: APIClient, today: String) {
        self.api = api
        self.today = today
    }

    var points: [TrendPoint] { TrendSeries.points(items, metric, span: span) }

    func load() async {
        isLoading = true
        message = nil
        defer { isLoading = false }

        do {
            items = try await api.listDailyMetrics(from: shiftDays(today, -span.days), to: today)
        } catch {
            // API が返した理由をそのまま見せる。何が悪いか分からないと直せない
            message = (error as? APIError)?.errorDescription ?? error.localizedDescription
        }
    }
}
