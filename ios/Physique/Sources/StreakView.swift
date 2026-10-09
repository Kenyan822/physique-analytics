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
            // **カレンダーから降りられる**（#276）。日を押すとその日の中身が出る
            .sheet(isPresented: Binding(
                get: { model.picked != nil },
                set: { if !$0 { model.closePicked() } }
            )) {
                if let d = model.picked { daySheet(d) }
            }
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
        let date = model.date(ofDay: n)
        let d = model.day(date)

        return Button {
            Task { await model.pick(date) }
        } label: {
            cellBody(n, d)
        }
        .buttonStyle(.plain)
        .accessibilityIdentifier("streakDay")
    }

    private func cellBody(_ n: Int, _ d: StreakDay?) -> some View {
        VStack(spacing: 2) {
            Text("\(n)").font(.caption2).monospacedDigit()
            HStack(spacing: 2) {
                // **判定できない日は薄いグレー。** 未達と見分けられるようにする
                dot(met: d?.mealGoalMet, on: .orange)
                dot(met: d?.trained, on: .blue)
            }
        }
        .frame(height: 34)
        .contentShape(Rectangle())
    }

    /// その日の中身（#276）。**なぜ未達だったかまで出す**
    private func daySheet(_ d: DayDetail) -> some View {
        NavigationStack {
            Form {
                Section("食事") {
                    macroRow("実績", d.meals.consumed)
                    if let t = d.meals.target {
                        macroRow("目標", t)
                    } else {
                        Text("この日の目標が決まっていない")
                            .font(.caption).foregroundStyle(.secondary)
                    }

                    // **未達の理由を出す。** 「✗」だけでは何を直せばいいか分からない
                    ForEach(d.meals.reasons, id: \.self) { r in
                        Label(r, systemImage: "exclamationmark.triangle")
                            .font(.caption).foregroundStyle(.orange)
                    }
                    if d.meals.goalMet == true {
                        Label("達成", systemImage: "checkmark.circle.fill")
                            .font(.caption).foregroundStyle(.green)
                    }
                }

                Section("筋トレ") {
                    if let w = d.workout {
                        LabeledContent(w.templateName) {
                            Text("\(w.setCount)セット").monospacedDigit()
                        }
                        ForEach(w.exercises ?? []) { e in
                            LabeledContent(e.exerciseName) {
                                Text(e.topWeightKg.map { "\(numberText($0))kg × \(e.setCount)" }
                                     ?? "\(e.setCount)セット")
                                    .font(.caption).monospacedDigit().foregroundStyle(.secondary)
                            }
                        }
                    } else {
                        Text("記録なし").font(.caption).foregroundStyle(.secondary)
                    }
                }

                Section("体組成") {
                    if let b = d.body, b.weightKg != nil || b.bodyFatPct != nil {
                        if let w = b.weightKg {
                            LabeledContent("体重") { Text("\(numberText(w))kg").monospacedDigit() }
                        }
                        if let f = b.bodyFatPct {
                            LabeledContent("体脂肪率") { Text("\(numberText(f))%").monospacedDigit() }
                        }
                    } else {
                        Text("記録なし").font(.caption).foregroundStyle(.secondary)
                    }
                }
            }
            .navigationTitle(JST.displayString(from: d.date))
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { model.closePicked() }
                }
            }
        }
        .presentationDetents([.medium, .large])
    }

    private func macroRow(_ title: String, _ m: Macros) -> some View {
        LabeledContent(title) {
            Text("P\(numberText(Double(m.proteinG))) F\(numberText(Double(m.fatG))) "
                 + "C\(numberText(Double(m.carbG)))  \(Int(m.kcal))kcal")
                .font(.caption).monospacedDigit()
        }
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
