import SwiftUI

/// 食事の記録（要件 N-01 / N-02 / N-05）。
///
/// **食事の直後や移動中に片手で触る。** 過去の記録から選べば入力が終わるのが
/// 一番速い経路で、手打ちはその次。
struct MealView: View {
    @State private var model: MealModel
    @State private var suggestQuery = ""
    @FocusState private var nameFocused: Bool

    init(api: APIClient) {
        _model = State(initialValue: MealModel(api: api))
    }

    var body: some View {
        NavigationStack {
            Form {
                summary
                slotPicker
                input
                recorded
            }
            .navigationTitle("食事")
            .task { await model.load() }
            .refreshable { await model.load() }
        }
    }

    // MARK: - 合計と残量

    @ViewBuilder
    private var summary: some View {
        Section {
            HStack {
                Total(label: "kcal", value: model.totals.kcal, remaining: model.remaining?.kcal)
                Divider()
                Total(label: "P", value: Int(model.totals.proteinG),
                      remaining: model.remaining.map { Int($0.proteinG) })
                Divider()
                Total(label: "F", value: Int(model.totals.fatG),
                      remaining: model.remaining.map { Int($0.fatG) })
                Divider()
                Total(label: "C", value: Int(model.totals.carbG),
                      remaining: model.remaining.map { Int($0.carbG) })
            }

            if let message = model.targetsMessage {
                // **目標が出せなくても記録はできる。** 理由だけ見せる
                Text(message).font(.caption).foregroundStyle(.orange)
            }
            if model.totals.withoutMacros > 0 {
                Text("PFC 未入力が \(model.totals.withoutMacros) 件。合計はその分少ない")
                    .font(.caption).foregroundStyle(.secondary)
            }
        } header: {
            Text(model.remaining == nil ? "今日の合計" : "今日の合計 / 残り")
        }
    }

    private var slotPicker: some View {
        Section {
            Picker("区分", selection: $model.slot) {
                ForEach(MealSlot.allCases) { Text($0.rawValue).tag($0) }
            }
            .pickerStyle(.segmented)
        }
    }

    // MARK: - 入力

    @ViewBuilder
    private var input: some View {
        Section("記録する") {
            TextField("食べたもの", text: $model.draft.name)
                .focused($nameFocused)
                .onChange(of: model.draft.name) { _, new in
                    suggestQuery = new
                    Task { await model.loadSuggestions(query: new) }
                }

            // 過去の記録から選ぶ（要件 N-02）。**選んだ時点で入力が終わる**
            if nameFocused, !model.suggestions.isEmpty {
                ForEach(model.suggestions) { s in
                    Button { model.pick(s) } label: {
                        HStack {
                            Text(s.name)
                            Spacer()
                            if let kcal = s.kcal {
                                Text("\(kcal) kcal").font(.caption).foregroundStyle(.secondary)
                            }
                            Text("\(s.count)回").font(.caption2).foregroundStyle(.tertiary)
                        }
                    }
                }
            }

            TextField("量（1個 / 200g）", text: $model.draft.qty)

            HStack {
                Num(label: "kcal", text: $model.draft.kcal)
                Num(label: "P", text: $model.draft.proteinG)
                Num(label: "F", text: $model.draft.fatG)
                Num(label: "C", text: $model.draft.carbG)
            }

            Button {
                nameFocused = false
                Task { await model.record() }
            } label: {
                Text(model.isWorking ? "記録中…" : "\(model.slot.rawValue)に記録")
                    .bold().frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(model.draft.isEmpty || model.isWorking)

            if let e = model.errorMessage {
                Text(e).font(.caption).foregroundStyle(.red)
            }
        }
    }

    // MARK: - 今日の記録

    @ViewBuilder
    private var recorded: some View {
        if !model.meals.isEmpty {
            Section("今日の記録") {
                ForEach(model.meals) { meal in
                    HStack {
                        VStack(alignment: .leading, spacing: 2) {
                            Text(meal.name)
                            if let slot = meal.slot?.rawValue, let qty = meal.qty {
                                Text("\(slot) / \(qty)").font(.caption).foregroundStyle(.secondary)
                            } else if let slot = meal.slot?.rawValue {
                                Text(slot).font(.caption).foregroundStyle(.secondary)
                            }
                        }
                        Spacer()
                        if let kcal = meal.kcal {
                            Text("\(kcal)").monospacedDigit()
                        }
                    }
                    .swipeActions {
                        Button("削除", role: .destructive) {
                            Task { await model.delete(meal) }
                        }
                    }
                }
            }
        }
    }
}

private struct Total: View {
    let label: String
    let value: Int
    let remaining: Int?

    var body: some View {
        VStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.secondary)
            Text("\(value)").font(.headline).monospacedDigit()
            if let r = remaining {
                // 超えたら負のまま出す。あとどれだけ削るかが分かる
                Text(r >= 0 ? "残 \(r)" : "超 \(-r)")
                    .font(.caption2)
                    .foregroundStyle(r >= 0 ? Color.secondary : Color.orange)
                    .monospacedDigit()
            }
        }
        .frame(maxWidth: .infinity)
    }
}

private struct Num: View {
    let label: String
    @Binding var text: String

    var body: some View {
        VStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.secondary)
            TextField("", text: $text)
                .keyboardType(.decimalPad)
                .multilineTextAlignment(.center)
                .textFieldStyle(.roundedBorder)
        }
    }
}
