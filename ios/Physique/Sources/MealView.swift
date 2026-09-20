import SwiftUI

/// 食事の記録（要件 N-01 / N-02 / N-05）。
///
/// **食事の直後や移動中に片手で触る。** 過去の記録から選べば入力が終わるのが
/// 一番速い経路で、手打ちはその次。
struct MealView: View {
    @State private var model: MealModel
    @State private var pickingDate = false
    @State private var editingTarget = false
    @FocusState private var focus: Field?

    /// 入力欄の並び。**キーボードの「次へ」がこの順に送る**
    private enum Field: Int, CaseIterable {
        case protein, fat, carb
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
            .keyboardFocusBar(
                focus: $focus, order: Field.allCases,
                // **キーボードの上から記録できる。** 打ってそのまま押せる形が
                // この画面の狙いに合う（#202）
                action: model.draft.isEmpty ? nil
                    : ("記録", { Task { await model.record() } })
            )
            .sheet(isPresented: $editingTarget) {
                targetSheet
            }
            // **.task(id:) は表示時にも走る。** 素の .task と併用すると
            // 開くたびに2回読みに行く
            .task(id: model.date) { await model.load() }
            .task { await model.loadManualTarget() }
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

    // MARK: - 目標（#195）

    /// **自動計算に任せるか、自分で決めるか。** 体重トレンドから逆算する
    /// 仕組み（A-02）はあるが、フェーズを登録するまで何も出ない
    private var targetSheet: some View {
        NavigationStack {
            Form {
                Section {
                    HStack(spacing: 12) {
                        Num(label: "P", text: $model.manualDraft.proteinG)
                            .accessibilityIdentifier("targetP")
                        Num(label: "F", text: $model.manualDraft.fatG)
                            .accessibilityIdentifier("targetF")
                        Num(label: "C", text: $model.manualDraft.carbG)
                            .accessibilityIdentifier("targetC")
                    }
                    LabeledContent("カロリー") {
                        Text(model.manualDraft.kcal.map { "\($0) kcal" } ?? "— kcal")
                            .monospacedDigit()
                            .foregroundStyle(model.manualDraft.kcal == nil ? .tertiary : .primary)
                    }
                } header: {
                    Text("1日の目標")
                } footer: {
                    Text(model.targetIsManual
                         ? "この値を使っている。消すと体重トレンドからの自動計算に戻る"
                         : "入れるとこの値が使われる。空のままなら体重トレンドから自動で決まる")
                }

                if let e = model.errorMessage {
                    ErrorNote(e)
                }

                if model.targetIsManual {
                    Section {
                        Button("自動計算に戻す", role: .destructive) {
                            Task {
                                await model.clearManualTarget()
                                editingTarget = false
                            }
                        }
                        .disabled(model.isWorking)
                    }
                }
            }
            .navigationTitle("目標")
            .navigationBarTitleDisplayMode(.inline)
            .dismissesKeyboardOnTap()
            .keyboardDoneButton()
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { editingTarget = false }
                }
                // **保存はツールバーに置く。** Form の下の方に置くと、
                // キーボードが出た時点で画面外に落ちて押せなくなる
                ToolbarItem(placement: .confirmationAction) {
                    Button(model.isWorking ? "保存中…" : "保存") {
                        Task {
                            await model.saveManualTarget()
                            if model.errorMessage == nil { editingTarget = false }
                        }
                    }
                    .disabled(model.isWorking)
                }
            }
        }
        // 伸ばせるようにする。キーボードが出ると medium では足りない
        .presentationDetents([.medium, .large])
    }

    // MARK: - 合計と残量

    /// **目標そのものをタップして直す。** 「目標を決める」ボタンを別に置くと、
    /// 数字を見て直したくなった場所と押す場所が離れる。
    @ViewBuilder
    private var summary: some View {
        Section {
            Button {
                editingTarget = true
            } label: {
                VStack(spacing: 8) {
                    // 1行目 —— 目標。ここが編集の入口
                    HStack(spacing: 6) {
                        Text(targetHeading)
                            .font(.caption).foregroundStyle(.secondary)
                        Spacer()
                        Image(systemName: "chevron.right")
                            .font(.caption2).foregroundStyle(.tertiary)
                    }

                    HStack {
                        Goal(label: "カロリー", value: model.target.map { Int($0.kcal) }, unit: "")
                        Divider()
                        Goal(label: "P", value: model.target.map { Int($0.proteinG) }, unit: "g")
                        Divider()
                        Goal(label: "F", value: model.target.map { Int($0.fatG) }, unit: "g")
                        Divider()
                        Goal(label: "C", value: model.target.map { Int($0.carbG) }, unit: "g")
                    }
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)

            // **目標が出せなくても記録はできる。** 理由だけ見せる
            if let message = model.targetsMessage {
                Text(message).font(.caption).foregroundStyle(.orange)
            }
        }

        Section {
            HStack {
                Total(label: "カロリー", value: model.totals.kcal, remaining: model.remaining?.kcal)
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

            if model.totals.withoutMacros > 0 {
                Text("PFC 未入力が \(model.totals.withoutMacros) 件。合計はその分少ない")
                    .font(.caption).foregroundStyle(.secondary)
            }
        } header: {
            Text(model.remaining == nil ? "今日の合計" : "今日の合計 / 残り")
        }
    }

    /// **未設定のときも「目標」だけ。** `›` が押せることを示していて、
    /// 値が「—」なら未設定だと見れば分かる
    private var targetHeading: String {
        guard model.target != nil else { return "目標" }

        return model.targetIsManual ? "目標（手動）" : "目標（自動計算）"
    }

    // MARK: - 入力

    /// **PFC を先頭に置く。** 入力の主目的がこれで、名前と量は後回しでよい（#188）。
    @ViewBuilder
    private var input: some View {
        Section {
            HStack(spacing: 12) {
                Num(label: "P", text: $model.draft.proteinG).focused($focus, equals: .protein)
                    .accessibilityIdentifier("draftP")
                Num(label: "F", text: $model.draft.fatG).focused($focus, equals: .fat)
                    .accessibilityIdentifier("draftF")
                Num(label: "C", text: $model.draft.carbG).focused($focus, equals: .carb)
                    .accessibilityIdentifier("draftC")
            }

            // 打ちながら見える。**送らない** —— サーバが同じ式で計算する
            LabeledContent("カロリー") {
                Text(model.draft.kcal.map { "\($0) kcal" } ?? "— kcal")
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
            .accessibilityIdentifier("recordButton")

            // **編集中のエラーはここに出さない。** 直しているのは下の行なのに、
            // 上の「記録する」に赤字が出ると、どこで何が起きたか分からない
            if model.editingID == nil, let e = model.errorMessage {
                ErrorNote(e)
            }
        } header: {
            Text("記録する")
        }

        // 「食べたもの」「量」は入力から外した（#188）。**PFC を打つのが主目的**で、
        // 名前は思い出せないことも多い。後から足したければ行をタップして直せる。
        //
        // 過去の記録から選ぶ（N-02）もここに付いていたので、いまは使えない。
        // 引数つきの食品マスタ（#201）で作り直す
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

        LabeledContent("カロリー") {
            Text(model.editDraft.kcal.map { "\($0) kcal" } ?? "— kcal")
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

/// 目標の1項目。**未設定は「—」**で、0 と区別する
private struct Goal: View {
    let label: String
    let value: Int?
    let unit: String

    var body: some View {
        VStack(spacing: 2) {
            Text(label).font(.caption2).foregroundStyle(.tertiary)
            Text(value.map { "\($0)\(unit)" } ?? "—")
                .font(.subheadline).monospacedDigit()
                .foregroundStyle(value == nil ? .tertiary : .secondary)
        }
        .frame(maxWidth: .infinity)
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
