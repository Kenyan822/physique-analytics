import SwiftUI

/// ジムでの入力画面（要件 T-01〜T-04）。
///
/// **片手・手袋・汗の状態で使える**ことが最優先。数値はキーボードではなく
/// ステッパーで刻み、RIR は1タップで選ぶ。
struct LogView: View {
    @State private var model: LogModel
    @State private var addingExercise = false
    @State private var changingDay = false
    @State private var pickingDate = false
    /// 直している記録済みセット（#247）
    @State private var editing: PendingSet?
    @FocusState private var focus: Field?

    /// 入力欄の並び。キーボードの「次へ」がこの順に送る
    private enum Field: Int, CaseIterable { case weight, reps }

    init(api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL)) {
        _model = State(initialValue: LogModel(api: api))
    }

    var body: some View {
        NavigationStack {
            Form {
                if !model.pendingMessage.isEmpty {
                    Section {
                        Text(model.pendingMessage)
                            .font(.footnote)
                            .foregroundStyle(.orange)
                    }
                }

                // **セットは種目の下に出る**（#247）。ここに一覧を出すと
                // 同じものが2か所に並び、どちらを直すのか分からなくなる
                todaySection
            }
            .navigationBarTitleDisplayMode(.inline)
            // **食事と同じ形**（#193 と揃える）。中央上部に置き、左右で1日ずつ
            .toolbar { ToolbarItem(placement: .principal) { dateNav } }
            .dismissesKeyboardOnTap()
            .keyboardDoneButton()
            .task { await model.load() }
            .onChange(of: model.selectedExerciseId) { _, id in
                Task { await model.selectExercise(id) }
            }
            // **カテゴリ別に全種目。** 今日の想定に無いものをその日だけ足す（#232）
            .sheet(isPresented: $addingExercise) { addExerciseSheet }
            .sheet(isPresented: $changingDay) { changeDaySheet }
            .sheet(item: $editing) { editSetSheet($0) }
            .alert("エラー", isPresented: $model.showError) {
                Button("閉じる", role: .cancel) {}
            } message: {
                Text(model.errorMessage)
            }
        }
    }

    /// **中央上部に置く。** 左右で1日ずつ、タップで任意の日へ（食事と同じ）
    private var dateNav: some View {
        HStack(spacing: 8) {
            Button { Task { await model.goToPreviousDay() } } label: {
                Image(systemName: "chevron.left")
            }

            Button { pickingDate = true } label: {
                Text(model.dateLabel).font(.headline)
            }
            .buttonStyle(.plain)

            // **未来には進めない。** 記録できない日を開いても意味が無い
            Button { Task { await model.goToNextDay() } } label: {
                Image(systemName: "chevron.right")
            }
            .disabled(!model.canGoNext)
        }
        .sheet(isPresented: $pickingDate) {
            NavigationStack {
                DatePicker("日付", selection: pickedDate, displayedComponents: .date)
                    .datePickerStyle(.graphical)
                    .padding()
                    .navigationTitle("日付")
                    .navigationBarTitleDisplayMode(.inline)
                    .toolbar {
                        ToolbarItem(placement: .cancellationAction) {
                            Button("閉じる") { pickingDate = false }
                        }
                    }
            }
            .presentationDetents([.medium])
        }
    }

    /// モデルは "YYYY-MM-DD" で持っているので載せ替える（食事と同じ）
    private var pickedDate: Binding<Date> {
        Binding(
            get: { JST.date(from: model.date) ?? Date() },
            set: { d in
                Task { await model.goTo(JST.dateString(from: d)) }
                pickingDate = false
            }
        )
    }

    /// 全種目をカテゴリ別に出す（#232）。**その日だけ足す。** ルーティンは変わらない。
    private var addExerciseSheet: some View {
        NavigationStack {
            List {
                ForEach(MuscleGroup.allCases, id: \.self) { g in
                    let items = model.exercises.filter { $0.muscleGroup == g }
                    if !items.isEmpty {
                        Section(g.rawValue) {
                            ForEach(items) { e in
                                Button {
                                    addingExercise = false
                                    Task { await model.addExercise(e.id) }
                                } label: {
                                    HStack {
                                        Text(e.name)
                                        if model.isDoneToday(e.id) {
                                            Image(systemName: "checkmark.circle.fill")
                                                .font(.caption).foregroundStyle(.green)
                                        }
                                        Spacer()
                                    }
                                    .contentShape(Rectangle())
                                }
                                .buttonStyle(.plain)
                            }
                        }
                    }
                }
            }
            .navigationTitle("種目を足す")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { addingExercise = false }
                }
            }
        }
    }

    /// Day を手でずらす（#232）。**予定が乱れた日に使う。**
    @ViewBuilder
    private var changeDaySheet: some View {
        NavigationStack {
            List {
                Section {
                    Button("自動に戻す") {
                        model.pickDay(nil)
                        changingDay = false
                    }
                } footer: {
                    Text("やった日だけ進む。前回の次が自動で選ばれる")
                }

                Section("直接選ぶ") {
                    ForEach(model.allDays) { d in
                        Button {
                            model.pickDay(d)
                            changingDay = false
                        } label: {
                            HStack {
                                Text("\(d.dayOrder)日目  \(d.templateName)")
                                Spacer()
                                if model.currentDay?.templateId == d.templateId {
                                    Image(systemName: "checkmark").font(.caption)
                                }
                            }
                            .contentShape(Rectangle())
                        }
                        .buttonStyle(.plain)
                    }
                }
            }
            .navigationTitle("今日やる日")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { changingDay = false }
                }
            }
        }
    }

    /// 今日やる種目（要件 T-01 / #232 / #247）。
    ///
    /// **ルーティンはプレースホルダ。** 並べ替え・追加・削除はその日だけに効き、
    /// ルーティン自体は変わらない（#242）。
    @ViewBuilder
    private var todaySection: some View {
        Section {
            ForEach(model.rows) { row in
                Button {
                    model.toggleExercise(row.exerciseId)
                } label: {
                    exerciseRow(row)
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("routineRow")

                // **その行の下に開く。** 画面の一番下に入力欄があると、
                // どの種目を打っているのか分からなくなる（#232）
                if model.selectedExerciseId == row.exerciseId {
                    setList(for: row.exerciseId)
                    inlineEditor
                }
            }
            .onMove { from, to in Task { await model.moveRows(from: from, to: to) } }
            .onDelete { offsets in Task { await model.removeRows(offsets) } }

            Button {
                addingExercise = true
            } label: {
                Label("種目を足す", systemImage: "plus")
            }
            .accessibilityIdentifier("addExercise")
        } header: {
            HStack {
                Text(model.dayLabel ?? "今日")
                Spacer()
                if model.savingRows {
                    ProgressView().controlSize(.mini)
                }
                if !model.allDays.isEmpty {
                    Button("変更") { changingDay = true }
                        .font(.caption)
                        .accessibilityIdentifier("changeDay")
                }
            }
        } footer: {
            if !model.rows.isEmpty {
                Text("長押しで並べ替え・左スワイプで削除。ルーティンは変わらない")
                    .font(.caption2)
            }
        }
    }

    /// 1行。ルーティン由来なら目標、手で足したなら部位を添える
    private func exerciseRow(_ row: LogModel.Row) -> some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(row.name)
                    if model.isDoneToday(row.exerciseId) {
                        Image(systemName: "checkmark.circle.fill")
                            .font(.caption).foregroundStyle(.green)
                    }
                }
                Text(row.target ?? row.muscleGroup.rawValue)
                    .font(.caption).foregroundStyle(.secondary).monospacedDigit()
            }

            Spacer()

            if model.selectedExerciseId == row.exerciseId {
                Image(systemName: "chevron.down")
                    .font(.caption).foregroundStyle(.tertiary)
            }
        }
        .contentShape(Rectangle())
    }

    /// 記録済みのセット。**1セット = 1行。** 押すと直せる、左スワイプで消せる（#247）
    @ViewBuilder
    private func setList(for exerciseId: UUID) -> some View {
        ForEach(model.sets(of: exerciseId)) { s in
            Button {
                editing = s
            } label: {
                HStack(spacing: 10) {
                    Text("\(s.setNo)")
                        .font(.caption).foregroundStyle(.secondary)
                        .frame(width: 16, alignment: .trailing)
                    Text(setLabel(s)).monospacedDigit()
                    Spacer()
                    Image(systemName: "checkmark.circle.fill")
                        .font(.caption).foregroundStyle(.green)
                }
                .contentShape(Rectangle())
            }
            .buttonStyle(.plain)
            .accessibilityIdentifier("loggedSet")
            .swipeActions {
                Button("削除", role: .destructive) {
                    Task { await model.deleteSet(id: s.id) }
                }
            }
        }
    }

    private func setLabel(_ s: PendingSet) -> String {
        let w = s.weightKg == s.weightKg.rounded() ? String(Int(s.weightKg)) : String(s.weightKg)

        return "\(w)kg × \(s.reps)回" + (s.rir.map { "  RIR\($0)" } ?? "")
    }

    /// 開いた行の下に出す入力欄（#232）。
    ///
    /// **数字で打てる。** ± だけだと 80kg にするのに32回押すことになる。
    @ViewBuilder
    private var inlineEditor: some View {
        VStack(spacing: 10) {
            // **前回を目の前に置く。** 超えられるかをその場で決める（要件 T-09）
            if model.loadingLast {
                line("前回", "取得中…")
            } else if model.last?.date != nil {
                line(
                    "前回",
                    model.describeLastSets()
                        + (model.last?.estimatedOneRm.map { String(format: "   1RM %.1f", $0) } ?? "")
                )
            } else {
                line("前回", "この種目は初回")
            }

            HStack(spacing: 8) {
                Text("重量").font(.caption).foregroundStyle(.secondary).frame(width: 40, alignment: .leading)
                Button { model.bumpWeight(-LogModel.weightStep) } label: {
                    Image(systemName: "minus.circle")
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("weightMinus")

                TextField("", text: $model.weightText)
                    .keyboardType(.decimalPad)
                    .multilineTextAlignment(.center)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 72)
                    .focused($focus, equals: .weight)
                    .accessibilityIdentifier("weightInput")

                Text("kg").font(.caption).foregroundStyle(.secondary)

                Button { model.bumpWeight(LogModel.weightStep) } label: {
                    Image(systemName: "plus.circle")
                }
                .buttonStyle(.plain)
                .accessibilityIdentifier("weightPlus")

                Spacer()
            }

            HStack(spacing: 8) {
                Text("レップ").font(.caption).foregroundStyle(.secondary).frame(width: 40, alignment: .leading)
                Button { model.bumpReps(-1) } label: { Image(systemName: "minus.circle") }
                    .buttonStyle(.plain)

                TextField("", text: $model.repsText)
                    .keyboardType(.numberPad)
                    .multilineTextAlignment(.center)
                    .textFieldStyle(.roundedBorder)
                    .frame(width: 72)
                    .focused($focus, equals: .reps)
                    .accessibilityIdentifier("repsInput")

                Text("回").font(.caption).foregroundStyle(.secondary)

                Button { model.bumpReps(1) } label: { Image(systemName: "plus.circle") }
                    .buttonStyle(.plain)

                Spacer()

                // RIR が無いと推定1RMが出せず進捗が測れない（openapi.yaml）
                Picker("RIR", selection: $model.rir) {
                    Text("RIR —").tag(Int?.none)
                    ForEach(0...10, id: \.self) { n in
                        Text("RIR \(n)").tag(Int?.some(n))
                    }
                }
                .pickerStyle(.menu)
            }

            Button {
                focus = nil
                Task { await model.record() }
            } label: {
                Text(model.recording ? "記録中…" : "\(model.nextSetNo)セット目を記録")
                    .bold().frame(maxWidth: .infinity)
            }
            .buttonStyle(.borderedProminent)
            .disabled(model.recording)
            .accessibilityIdentifier("recordSet")
        }
        .padding(.vertical, 4)
    }


    /// 記録済みセットを直すシート（#247）。
    ///
    /// **その場で直さずシートにする。** 行の中に TextField を置くと、
    /// 並べ替えのドラッグと取り合いになる
    private func editSetSheet(_ s: PendingSet) -> some View {
        EditSetSheet(set: s) { w, r, rir in
            await model.updateSet(id: s.id, weightKg: w, reps: r, rir: rir)
            editing = nil
        } onDelete: {
            await model.deleteSet(id: s.id)
            editing = nil
        }
    }

    /// 見出し + 中身の1行。**前回の表示に使う**
    private func line(_ label: String, _ value: String) -> some View {
        HStack(spacing: 8) {
            Text(label).font(.caption).foregroundStyle(.secondary)
                .frame(width: 40, alignment: .leading)
            Text(value).font(.caption).foregroundStyle(.secondary).monospacedDigit()
            Spacer()
        }
    }



}

#Preview {
    LogView()
}
