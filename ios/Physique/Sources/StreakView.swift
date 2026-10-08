import SwiftUI

/// 続いているかを一目で見る（要件 A-xx / #249）。
///
/// **数字ではなくマークで見せる。** 細かい値は食事・記録の各画面にある。
struct StreakView: View {
    @State private var model: StreakModel

    private static let weekdays = ["日", "月", "火", "水", "木", "金", "土"]
    private let columns = Array(repeating: GridItem(.flexible(), spacing: 4), count: 7)

    init(api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL)) {
        _model = State(initialValue: StreakModel(api: api))
    }

    var body: some View {
        NavigationStack {
            Form {
                Section {
                    HStack {
                        streakCount("食事", model.mealStreak, systemImage: "fork.knife", tint: .orange)
                        Divider()
                        streakCount("筋トレ", model.trainedStreak, systemImage: "dumbbell", tint: .blue)
                    }
                } footer: {
                    Text("今日から遡った連続日数。食事は**判定できない日では切れない**")
                }

                Section {
                    calendar
                } header: {
                    HStack {
                        Text(model.monthLabel)
                        Spacer()
                        if model.loading { ProgressView().controlSize(.mini) }
                    }
                } footer: {
                    legend
                }
            }
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .principal) { monthNav } }
            .task { await model.load() }
            .alert("エラー", isPresented: $model.showError) {
                Button("閉じる", role: .cancel) {}
            } message: {
                Text(model.errorMessage)
            }
        }
    }

    /// 食事と同じ形（#193）。中央上部に置き、左右で1ヶ月ずつ
    private var monthNav: some View {
        HStack(spacing: 8) {
            Button { Task { await model.goToPreviousMonth() } } label: {
                Image(systemName: "chevron.left")
            }
            .accessibilityIdentifier("prevMonth")

            Text(model.monthLabel).font(.headline)

            // **未来には進めない。** 記録のしようがない月を見ても意味が無い
            Button { Task { await model.goToNextMonth() } } label: {
                Image(systemName: "chevron.right")
            }
            .disabled(!model.canGoNext)
            .accessibilityIdentifier("nextMonth")
        }
    }

    private func streakCount(
        _ title: String, _ n: Int, systemImage: String, tint: Color
    ) -> some View {
        VStack(spacing: 2) {
            Label(title, systemImage: systemImage)
                .font(.caption).foregroundStyle(.secondary)
            Text("\(n)").font(.title2).bold().monospacedDigit().foregroundStyle(tint)
            Text("日").font(.caption2).foregroundStyle(.secondary)
        }
        .frame(maxWidth: .infinity)
    }

    private var calendar: some View {
        LazyVGrid(columns: columns, spacing: 6) {
            ForEach(Self.weekdays, id: \.self) { w in
                Text(w).font(.caption2).foregroundStyle(.secondary)
            }
            ForEach(0..<model.leadingBlanks, id: \.self) { _ in Color.clear.frame(height: 34) }
            ForEach(1...model.daysInMonth, id: \.self) { n in
                cell(n)
            }
        }
        .accessibilityIdentifier("streakCalendar")
    }

    private func cell(_ n: Int) -> some View {
        let d = model.day(model.date(ofDay: n))

        return VStack(spacing: 2) {
            Text("\(n)").font(.caption2).monospacedDigit()
            HStack(spacing: 2) {
                // **判定できない日は薄いグレー。** 未達と見分けられるようにする
                dot(met: d?.mealGoalMet, on: .orange)
                dot(met: d?.trained, on: .blue)
            }
        }
        .frame(height: 34)
    }

    @ViewBuilder
    private func dot(met: Bool?, on color: Color) -> some View {
        switch met {
        case true: Circle().fill(color).frame(width: 6, height: 6)
        case false: Circle().strokeBorder(color.opacity(0.4), lineWidth: 1).frame(width: 6, height: 6)
        case nil: Circle().fill(Color.secondary.opacity(0.15)).frame(width: 6, height: 6)
        }
    }

    private var legend: some View {
        HStack(spacing: 10) {
            legendItem(.orange, "食事")
            legendItem(.blue, "筋トレ")
            HStack(spacing: 3) {
                Circle().strokeBorder(Color.secondary.opacity(0.4), lineWidth: 1)
                    .frame(width: 6, height: 6)
                Text("未達")
            }
            HStack(spacing: 3) {
                Circle().fill(Color.secondary.opacity(0.15)).frame(width: 6, height: 6)
                Text("判定できない")
            }
        }
        .font(.caption2)
    }

    private func legendItem(_ c: Color, _ label: String) -> some View {
        HStack(spacing: 3) {
            Circle().fill(c).frame(width: 6, height: 6)
            Text(label)
        }
    }
}
