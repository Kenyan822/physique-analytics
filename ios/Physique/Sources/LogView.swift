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

                todaySection

                // **ルーティンがあるときは行の下に開く**（#232）。
                // ここに出すと二重になり、どちらに打つのか分からなくなる
                if model.selectedExerciseId != nil, model.currentDay == nil {
                    lastPerformanceSection
                    inputSection
                }

                if !model.logged.isEmpty {
                    Section("今日の記録") {
                        ForEach(model.logged) { s in
                            Text(model.describe(s)).font(.callout).monospacedDigit()
                        }
                    }
                }
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
                                    model.addExercise(e.id)
                                    addingExercise = false
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

    /// 今日やる想定（要件 T-01 / #232）。
    ///
    /// **最初から並んでいる。** 以前は別画面で50種目から1つ選ばせていたが、
    /// ジムで毎セットこれをやるのは重い。
    ///
    /// ルーティンが未登録なら、今までどおり全種目から選ぶ。
    @ViewBuilder
    private var todaySection: some View {
        if let day = model.currentDay {
            Section {
                ForEach(day.items) { item in
                    Button {
                        model.toggleExercise(item.exerciseId)
                    } label: {
                        routineRow(item)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("routineRow")

                    // **その行の下に開く。** 画面の一番下に入力欄があると、
                    // どの種目を打っているのか分からなくなる（#232）
                    if model.selectedExerciseId == item.exerciseId {
                        inlineEditor
                    }
                }

                // **足した種目も行として並べる。** 開くだけだと入力欄が
                // ルーティンの行の下にしか出ず、何も起きない（実機で踏んだ）
                ForEach(model.extraExercises) { e in
                    Button {
                        model.toggleExercise(e.id)
                    } label: {
                        extraRow(e)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("routineRow")

                    if model.selectedExerciseId == e.id {
                        inlineEditor
                    }
                }

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
                    Button("変更") { changingDay = true }
                        .font(.caption)
                        .accessibilityIdentifier("changeDay")
                }
            }
        } else {
            // ルーティンが未登録。**記録はできる**ので今までどおり選ばせる
            Section("種目") {
                Picker("種目", selection: $model.selectedExerciseId) {
                    Text("選ぶ").tag(UUID?.none)
                    ForEach(model.exercises) { e in
                        Text("\(e.name)（\(e.muscleGroup.rawValue)）\(model.doneMark(e.id))")
                            .tag(UUID?.some(e.id))
                    }
                }
                .pickerStyle(.navigationLink)
            }
        }
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

    /// その日だけ足した種目の1行。**目標が無い**ので部位だけ添える
    private func extraRow(_ e: Exercise) -> some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(e.name)
                    if model.isDoneToday(e.id) {
                        Image(systemName: "checkmark.circle.fill")
                            .font(.caption).foregroundStyle(.green)
                    }
                }
                Text("\(e.muscleGroup.rawValue)   今日だけ")
                    .font(.caption).foregroundStyle(.secondary)
            }

            Spacer()

            if model.selectedExerciseId == e.id {
                Image(systemName: "chevron.down")
                    .font(.caption).foregroundStyle(.tertiary)
            }
        }
        .contentShape(Rectangle())
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

    /// 今日やる種目1行。**目標と前回を並べて、超えられるか即断できるようにする。**
    private func routineRow(_ item: RoutineDayItem) -> some View {
        HStack(spacing: 8) {
            VStack(alignment: .leading, spacing: 2) {
                HStack(spacing: 6) {
                    Text(item.exerciseName)
                    if model.isDoneToday(item.exerciseId) {
                        Image(systemName: "checkmark.circle.fill")
                            .font(.caption).foregroundStyle(.green)
                    }
                }
                Text("\(item.targetLabel)   \(item.lastLabel)")
                    .font(.caption).foregroundStyle(.secondary).monospacedDigit()
            }

            Spacer()

            if model.selectedExerciseId == item.exerciseId {
                Image(systemName: "chevron.down")
                    .font(.caption).foregroundStyle(.tertiary)
            }
        }
        .contentShape(Rectangle())
    }

    /// 前回の実施内容（要件 T-02 / T-09）。その場で超えられるか判断できるようにする。
    @ViewBuilder
    private var lastPerformanceSection: some View {
        Section("前回") {
            if model.loadingLast {
                Text("取得中…").foregroundStyle(.secondary)
            } else if let last = model.last, let date = last.date {
                HStack {
                    Text(JST.displayString(from: date))
                    Spacer()
                    if let e1rm = last.estimatedOneRm {
                        Text("推定1RM \(e1rm, specifier: "%.1f")kg")
                            .foregroundStyle(.secondary)
                            .monospacedDigit()
                    }
                }
                .font(.footnote)

                Text(model.describeLastSets())
                    .font(.callout)
                    .monospacedDigit()
            } else {
                Text("この種目は初回").foregroundStyle(.secondary)
            }
        }
    }

    /// ルーティンが未登録のときの入力欄。**中身はインラインと同じ。**
    private var inputSection: some View {
        Section("入力") { inlineEditor }
    }
}

#Preview {
    LogView()
}
