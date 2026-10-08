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
    /// 種目マスタに登録する（#264）
    @State private var registering = false
    @State private var newExerciseName = ""
    @State private var newExerciseGroup: MuscleGroup = .chest
    @State private var newExerciseCompound = false
    @FocusState private var focus: Field?

    /// 入力欄の位置。**行をまたいで移動できる**ように、どの行のどの欄かで持つ
    private struct Field: Hashable {
        enum Kind: Int, CaseIterable { case weight, reps, rir }

        let set: UUID
        let kind: Kind
    }

    /// キーボードの ↑↓ が送る順。行ごとに 重量 → レップ → RIR
    private var fieldOrder: [Field] {
        model.drafts.flatMap { d in Field.Kind.allCases.map { Field(set: d.id, kind: $0) } }
    }

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
            // 食事と同じ（#229）。**行をまたいで ↑↓ で移動できる**
            .keyboardFocusBar(focus: $focus, order: fieldOrder)
            // **欄から離れたら保存する。** 確定ボタンを置かない（#256）
            .onChange(of: focus) { old, _ in
                guard let old else { return }
                Task { await model.commitDraft(old.set) }
            }
            .task { await model.load() }
            .onChange(of: model.selectedExerciseId) { _, id in
                Task { await model.selectExercise(id) }
            }
            // **カテゴリ別に全種目。** 今日の想定に無いものをその日だけ足す（#232）
            .sheet(isPresented: $addingExercise) { addExerciseSheet }
            .sheet(isPresented: $changingDay) { changeDaySheet }
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
                                // **論理削除。** 記録は残る（#264）
                                .swipeActions {
                                    Button("消す", role: .destructive) {
                                        Task { await model.removeExerciseFromMaster(e.id) }
                                    }
                                }
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
                // **マスタに無い種目はここから登録する**（#264）。
                // ジムで思いついたときに2回探さなくて済む
                ToolbarItem(placement: .confirmationAction) {
                    Button("新規") { registering = true }
                        .accessibilityIdentifier("openExerciseRegister")
                }
            }
            .sheet(isPresented: $registering) { registerExerciseSheet }
        }
    }

    /// 種目マスタに登録する（#264）。
    ///
    /// **登録したらその日のリストにも入って開く。** 登録と「今日やる」を
    /// 別操作にすると、ジムで種目を思いついたときに2回探すことになる
    private var registerExerciseSheet: some View {
        NavigationStack {
            Form {
                Section {
                    LabeledContent("名前") {
                        TextField("ケーブルクロスオーバー", text: $newExerciseName)
                            .multilineTextAlignment(.trailing)
                            .accessibilityIdentifier("newExerciseName")
                    }

                    // **自由入力にしない。** 部位がずれると分析の部位別集計が壊れる
                    Picker("部位", selection: $newExerciseGroup) {
                        ForEach(MuscleGroup.allCases, id: \.self) { g in
                            Text(g.rawValue).tag(g)
                        }
                    }
                    .accessibilityIdentifier("newExerciseGroup")

                    Toggle("多関節", isOn: $newExerciseCompound)
                        .accessibilityIdentifier("newExerciseCompound")
                } footer: {
                    Text("多関節だとインターバルの既定が長くなる"
                         + "（\(LogModel.compoundRestSec / 60)分 / \(LogModel.isolationRestSec)秒）。"
                         + "**既にある種目を別表記で足さないこと。** 時系列が分断される")
                }
            }
            .navigationTitle("種目を登録")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .cancellationAction) {
                    Button("閉じる") { registering = false }
                }
                ToolbarItem(placement: .confirmationAction) {
                    Button("登録") {
                        Task {
                            await model.registerExercise(
                                name: newExerciseName, muscleGroup: newExerciseGroup,
                                isCompound: newExerciseCompound)
                            if !model.showError {
                                newExerciseName = ""
                                registering = false
                                addingExercise = false
                            }
                        }
                    }
                    .disabled(newExerciseName.trimmingCharacters(in: .whitespaces).isEmpty)
                    .accessibilityIdentifier("saveExercise")
                }
            }
        }
        .presentationDetents([.medium])
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
                    lastLine
                    setRows(for: row.exerciseId)
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

    /// セットの行（#247 / #251 / #256）。
    ///
    /// **記録済みも入力中も同じ行。** 確定ボタンを置かず、欄から離れた時点で
    /// 保存する。ジムで「記録を押し忘れて1セット消えた」が起きないようにする。
    @ViewBuilder
    private func setRows(for exerciseId: UUID) -> some View {
        ForEach(model.drafts) { d in
            HStack(spacing: 4) {
                Text("\(d.displayNo)")
                    .font(.caption).foregroundStyle(.secondary)
                    .frame(width: 16, alignment: .trailing)

                box(d, .weight, width: 50, pad: .decimalPad)
                Text("kg").font(.caption).foregroundStyle(.secondary)
                Text("×").font(.caption).foregroundStyle(.tertiary).padding(.horizontal, 2)
                box(d, .reps, width: 40, pad: .numberPad)
                Text("回").font(.caption).foregroundStyle(.secondary)
                Text("RIR").font(.caption).foregroundStyle(.secondary).padding(.leading, 6)
                box(d, .rir, width: 32, pad: .numberPad)

                Spacer(minLength: 4)
            }
            // **保存済みの印は出さない。** 欄から離れれば黙って保存される。
            // 未送信が溜まっていれば画面上部の「未送信 N 件」で分かる（#262）
            // **行に identifier を付けない。** 付けると子の TextField まで
            // それで上書きされ、weightInput などが引けなくなる
            .swipeActions {
                Button("削除", role: .destructive) {
                    Task { await model.deleteDraft(d.id) }
                }
            }
        }

        Button {
            commitFocused()
            model.addSetRow()
        } label: {
            Label("セットを足す", systemImage: "plus")
                .font(.callout)
        }
        .accessibilityIdentifier("addSet")
    }

    /// 数字の枠。**離れたら保存する**（確定ボタンを置かない）
    private func box(
        _ d: LogModel.SetDraft, _ kind: Field.Kind, width: CGFloat, pad: UIKeyboardType
    ) -> some View {
        TextField("", text: binding(d, kind))
            .keyboardType(pad)
            .multilineTextAlignment(.center)
            .textFieldStyle(.roundedBorder)
            .frame(width: width)
            .focused($focus, equals: Field(set: d.id, kind: kind))
            .accessibilityIdentifier(identifier(kind))
    }

    private func binding(_ d: LogModel.SetDraft, _ kind: Field.Kind) -> Binding<String> {
        switch kind {
        case .weight: Binding(get: { d.weightText }, set: { model.setWeight($0, for: d.id) })
        case .reps: Binding(get: { d.repsText }, set: { model.setReps($0, for: d.id) })
        case .rir: Binding(get: { d.rirText }, set: { model.setRir($0, for: d.id) })
        }
    }

    /// UI テストから引くため。**行が複数あるので firstMatch で使う**
    private func identifier(_ kind: Field.Kind) -> String {
        switch kind {
        case .weight: "weightInput"
        case .reps: "repsInput"
        case .rir: "rirInput"
        }
    }

    /// いま開いている行を保存する。欄を離れたときとキーボードを閉じたときに呼ぶ
    private func commitFocused() {
        guard let f = focus else { return }
        Task { await model.commitDraft(f.set) }
    }

    /// 前回この種目をやったセッション（#251）。**超えられるかをその場で決める**
    @ViewBuilder
    private var lastLine: some View {
        if model.loadingLast {
            line("前回", "取得中…")
        } else if let summary = model.lastSummary {
            line("前回", summary)
        } else {
            line("前回", "この種目は初回")
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
