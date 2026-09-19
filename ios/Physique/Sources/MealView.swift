import SwiftUI

/// 食事の記録（要件 N-01 / N-02 / N-05）。
///
/// **食事の直後や移動中に片手で触る。** 過去の記録から選べば入力が終わるのが
/// 一番速い経路で、手打ちはその次。
struct MealView: View {
    @State private var model: MealModel
    @State private var pickingDate = false
    @FocusState private var focus: Field?

    /// 入力欄の並び。**キーボードの「次へ」がこの順に送る**
    private enum Field: Int, CaseIterable {
        case protein, fat, carb, name, qty
        case editP, editF, editC, editName, editQty
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
            .navigationBarTitleDisplayMode(.inline)
            .toolbar { ToolbarItem(placement: .principal) { dateNav } }
            .dismissesKeyboardOnTap()
            .keyboardFocusBar(focus: $focus, order: Field.allCases)
            .task { await model.load() }
            .task(id: model.date) { await model.load() }
            .refreshable { await model.load() }
        }
    }

    // MARK: - 日付（#193）

    /// **中央上部に置く。** 左右で1日ずつ、タップで任意の日へ。
    private var dateNav: some View {
        HStack(spacing: 8) {
            Button { model.goToPreviousDay() } label: { Image(systemName: "chevron.left") }

            Button { pickingDate = true } label: {
                Text(model.dateLabel).font(.headline)
            }
            .buttonStyle(.plain)

            // **未来には進めない。** 記録できない日を開いても意味が無い
            Button { model.goToNextDay() } label: { Image(systemName: "chevron.right") }
                .disabled(!model.canGoNext)
        }
        // **シートで出す。** popover は iPhone だと幅が詰まって
        // カレンダーが読めない大きさになる
        .sheet(isPresented: $pickingDate) {
            NavigationStack {
                DatePicker(
                    "日付", selection: pickedDate,
                    in: ...Date(), displayedComponents: .date
                )
                .datePickerStyle(.graphical)
                .padding(.horizontal)
                .navigationTitle("日付を選ぶ")
                .navigationBarTitleDisplayMode(.inline)
                .toolbar {
                    ToolbarItem(placement: .cancellationAction) {
                        Button("閉じる") { pickingDate = false }
                    }
                }
            }
            // カレンダーが入る高さだけ出す。全画面にしない
            .presentationDetents([.medium])
        }
    }

    /// 編集中の時刻。モデルは "HH:MM" の文字列で持っているので載せ替える。
    ///
    /// **空だったら今の時刻から始める。** ホイールに「未設定」は無いので、
    /// 開いた時点で何らかの値が要る
    private var editTime: Binding<Date> {
        Binding(
            get: { JST.time(from: model.editTime) ?? Date() },
            set: { model.editTime = JST.timeString(from: $0) }
        )
    }

    /// `DatePicker` は `Date` を要求するが、モデルは JST の文字列で持っている
    private var pickedDate: Binding<Date> {
        Binding(
            get: { JST.date(from: model.date) ?? Date() },
            set: { model.goTo(JST.dateString(from: $0)); pickingDate = false }
        )
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
            LabeledContent("kcal") {
                Text(model.draft.kcal.map { "\($0) kcal" } ?? "—")
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

            // **編集中のエラーはここに出さない。** 直しているのは下の行なのに、
            // 上の「記録する」に赤字が出ると、どこで何が起きたか分からない
            if model.editingID == nil, let e = model.errorMessage {
                ErrorNote(e)
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
                    if model.editingID == meal.id {
                        editor
                    } else {
                        row(meal)
                            .contentShape(Rectangle())
                            // **その場で開く。** 別画面に飛ばすと、直すたびに
                            // 行き来することになる
                            .onTapGesture { model.beginEditing(meal) }
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

    private func row(_ meal: Meal) -> some View {
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
                // **単位を書く。** 数字だけだと、隣の PFC のグラム数と
                // 見分けが付かない
                if let kcal = meal.kcal {
                    HStack(spacing: 2) {
                        Text("\(kcal)").monospacedDigit().font(.subheadline)
                        Text("kcal").font(.caption2).foregroundStyle(.secondary)
                    }
                }
            }

            // 名前は任意。入っていれば2行目に出す
            if let name = meal.name {
                Text(meal.qty.map { "\(name) / \($0)" } ?? name)
                    .font(.caption).foregroundStyle(.secondary)
            }
        }
    }

    /// 行を開いたときの編集欄（#193）。
    ///
    /// **Form の行として出す。** VStack に TextField を並べると、
    /// 周りの行と余白も区切り線も揃わず、開いた場所だけ浮いて見える。
    @ViewBuilder
    private var editor: some View {
        // **文字で打たせない。** "19:40" の形を人に守らせる理由が無い。
        // ホイールなら不正な値が入らず、検証も要らなくなる
        DatePicker(
            "時刻", selection: editTime, displayedComponents: .hourAndMinute
        )

        HStack(spacing: 12) {
            Num(label: "P", text: $model.editDraft.proteinG).focused($focus, equals: .editP)
            Num(label: "F", text: $model.editDraft.fatG).focused($focus, equals: .editF)
            Num(label: "C", text: $model.editDraft.carbG).focused($focus, equals: .editC)
        }

        LabeledContent("kcal") {
            Text(model.editDraft.kcal.map { "\($0) kcal" } ?? "—")
                .monospacedDigit()
                .foregroundStyle(model.editDraft.kcal == nil ? .tertiary : .primary)
        }

        LabeledContent("食べたもの") {
            TextField("任意", text: $model.editDraft.name)
                .multilineTextAlignment(.trailing)
                .focused($focus, equals: .editName)
        }

        LabeledContent("量") {
            TextField("1個 / 200g", text: $model.editDraft.qty)
                .multilineTextAlignment(.trailing)
                .focused($focus, equals: .editQty)
        }

        // **直している行のすぐ下に出す。** どこで失敗したかが分かる
        if let e = model.errorMessage {
            ErrorNote(e)
        }

        HStack {
            Button("やめる", role: .cancel) { model.cancelEditing() }
                .buttonStyle(.bordered)
            Spacer()
            Button(model.isWorking ? "保存中…" : "保存") {
                focus = nil
                Task { await model.saveEdit() }
            }
            .buttonStyle(.borderedProminent)
            .disabled(model.isWorking)
        }
    }
}

/// エラーの出し方を1か所に揃える。
///
/// **記号を付ける。** 赤字だけだと、色が読み取れない環境で
/// ただの注記と区別が付かない
private struct ErrorNote: View {
    let text: String

    init(_ text: String) { self.text = text }

    var body: some View {
        Label(text, systemImage: "exclamationmark.triangle.fill")
            .font(.caption)
            .foregroundStyle(.red)
    }
}

/// 一覧に出す PFC。**未入力は「—」**で、0 と区別する
private struct Macro: View {
    let label: String
    let value: Double?

    var body: some View {
        HStack(spacing: 1) {
            Text(label).font(.caption2).foregroundStyle(.tertiary)
            Text(value.map { ($0 == $0.rounded() ? String(Int($0)) : String($0)) + "g" } ?? "—")
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
