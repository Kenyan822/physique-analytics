import SwiftUI

/// 食事の推移（要件 N-05 / #308）。
///
/// **数値の推移を見るだけの画面。** 目標との比較や判定はしない。
struct MealChartView: View {
    @State private var model: MealChartModel

    init(api: APIClient, today: String = JST.dateString()) {
        _model = State(initialValue: MealChartModel(api: api, today: today))
    }

    var body: some View {
        VStack(spacing: 12) {
            Picker("指標", selection: $model.metric) {
                ForEach(MealMetric.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
            .accessibilityIdentifier("metricPicker")

            Picker("粒度", selection: $model.span) {
                ForEach(TrendSpan.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
            .frame(maxWidth: 180)
            .accessibilityIdentifier("spanPicker")

            TrendChart(
                points: model.points,
                unit: model.metric.unit,
                isLoading: model.isLoading,
                emptyNote: "\(model.span.days)日さかのぼって記録が見つからない"
            )

            // **月は1日あたりの平均。** 月の合計と取り違えると桁が変わる
            if model.span == .monthly {
                Text("月は1日あたりの平均").font(.caption).foregroundStyle(.secondary)
            }

            if let message = model.message {
                Text(message).font(.caption).foregroundStyle(.secondary)
            }

            Spacer()
        }
        .padding()
        .navigationTitle("推移")
        .navigationBarTitleDisplayMode(.inline)
        .task(id: model.span) { await model.load() }
    }
}

/// 食事の推移の状態。
@Observable
@MainActor
final class MealChartModel {
    var metric: MealMetric = .kcal
    var span: TrendSpan = .daily

    private(set) var isLoading = false
    private(set) var message: String?

    private var meals: [Meal] = []
    private let api: APIClient
    private let today: String

    init(api: APIClient, today: String) {
        self.api = api
        self.today = today
    }

    var points: [TrendPoint] { TrendSeries.points(meals, metric, span: span) }

    func load() async {
        isLoading = true
        message = nil
        defer { isLoading = false }

        do {
            // **daily_metrics の kcal 列は書かれていない。** 食事の記録から合計する
            meals = try await api.listMeals(from: shiftDays(today, -span.days), to: today)
        } catch {
            message = (error as? APIError)?.errorDescription ?? error.localizedDescription
        }
    }
}
