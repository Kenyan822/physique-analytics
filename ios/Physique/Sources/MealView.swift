import SwiftUI

/// 食事の記録（要件 N-01 / N-02 / N-05）。
///
/// **食事の直後や移動中に片手で触る。** 過去の記録から選べば入力が終わるのが
/// 一番速い経路で、手打ちはその次。
struct MealView: View {
    @State private var model: MealModel
    @FocusState private var focus: Field?

    /// 入力欄の並び。**キーボードの「次へ」がこの順に送る**
    private enum Field: Int, CaseIterable {
        case protein, fat, carb, name, qty
    }

    init(api: APIClient) {
        _model = State(initialValue: MealModel(api: api))
    }

    var body: some View {
        NavigationStack {
            Form {
                summary
                input
                recorded
            }
            .navigationTitle("食事")
            .dismissesKeyboardOnTap()
            .keyboardFocusBar(focus: $focus, order: Field.allCases)
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

    // MARK: - 入力

    /// **PFC を先頭に置く。** 入力の主目的がこれで、名前と量は後回しでよい（#188）。
    @ViewBuilder
    private var input: some View {
        Section {
            HStack(spacing: 12) {
                Num(label: "P", text: $model.draft.proteinG).focused($focus, equals: .protein)
                Num(label: "F", text: $model.draft.fatG).focused($focus, equals: .fat)
                Num(label: "C", text: $model.draft.carbG).focused($focus, equals: .carb)
            }

            // 打ちながら見える。**送らない** —— サーバが同じ式で計算する
            HStack {
                Text("kcal").font(.caption).foregroundStyle(.secondary)
                Spacer()
                Text(model.draft.kcal.map(String.init) ?? "—")
                    .monospacedDigit()
                    .foregroundStyle(model.draft.kcal == nil ? .tertiary : .primary)
            }

            Button {
                focus = nil
                Task { await model.record() }
            } label: {
                Text(model.isWorking ? "記録中…" : "記録")
                    .bold().frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(model.draft.isEmpty || model.isWorking)

            if let e = model.errorMessage {
                Text(e).font(.caption).foregroundStyle(.red)
            }
        } header: {
            Text("記録する")
        }

        // **任意。** 思い出せないなら空のままでよい（#188）
        Section {
            TextField("食べたもの（任意）", text: $model.draft.name)
                .focused($focus, equals: .name)
                .onChange(of: model.draft.name) { _, new in
                    Task { await model.loadSuggestions(query: new) }
                }

            // 過去の記録から選ぶ（要件 N-02）。**選んだ時点で入力が終わる**
            if focus == .name, !model.suggestions.isEmpty {
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
                .focused($focus, equals: .qty)
        }
    }

    // MARK: - 今日の記録

    @ViewBuilder
    private var recorded: some View {
        if !model.meals.isEmpty {
            Section("今日の記録") {
                ForEach(model.meals) { meal in
                    VStack(alignment: .leading, spacing: 3) {
                        HStack(spacing: 10) {
                            // **時刻を先頭に。** 区分より「何時に食べたか」の方が
                            // 思い出しやすく、並び順とも一致する
                            Text(meal.at ?? "--:--")
                                .font(.subheadline).monospacedDigit()
                                .foregroundStyle(meal.at == nil ? .tertiary : .secondary)

                            // **PFC を一覧に出す。** 何を食べたかより、
                            // 何を摂ったかを見返すことの方が多い（#188）
                            Macro(label: "P", value: meal.proteinG)
                            Macro(label: "F", value: meal.fatG)
                            Macro(label: "C", value: meal.carbG)

                            Spacer()
                            if let kcal = meal.kcal {
                                Text("\(kcal)").monospacedDigit().font(.subheadline)
                            }
                        }

                        // 名前は任意。入っていれば2行目に出す
                        if let name = meal.name {
                            Text(meal.qty.map { "\(name) / \($0)" } ?? name)
                                .font(.caption).foregroundStyle(.secondary)
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

/// 一覧に出す PFC。**未入力は「—」**で、0 と区別する
private struct Macro: View {
    let label: String
    let value: Double?

    var body: some View {
        HStack(spacing: 1) {
            Text(label).font(.caption2).foregroundStyle(.tertiary)
            Text(value.map { $0 == $0.rounded() ? String(Int($0)) : String($0) } ?? "—")
                .font(.caption).monospacedDigit()
                .foregroundStyle(value == nil ? .tertiary : .secondary)
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
