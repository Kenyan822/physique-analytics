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

                if model.selectedExerciseId != nil {
                    lastPerformanceSection
                    inputSection
                    recordSection
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
                                    model.selectedExerciseId = e.id
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
                        model.selectedExerciseId = item.exerciseId
                    } label: {
                        routineRow(item)
                    }
                    .buttonStyle(.plain)
                    .accessibilityIdentifier("routineRow")
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

    private var inputSection: some View {
        Section("入力") {
            Stepper(value: $model.weightKg, in: 0...500, step: LogModel.weightStep) {
                LabeledContent("重量") {
                    Text("\(model.weightKg, specifier: "%g") kg").monospacedDigit()
                }
            }

            Stepper(value: $model.reps, in: 0...100) {
                LabeledContent("レップ") {
                    Text("\(model.reps) 回").monospacedDigit()
                }
            }

            // RIR が無いと推定1RMが計算できず進捗が測れない（openapi.yaml）
            Picker("RIR", selection: $model.rir) {
                Text("—").tag(Int?.none)
                ForEach(0...10, id: \.self) { n in
                    Text("\(n)").tag(Int?.some(n))
                }
            }
            .pickerStyle(.menu)
        }
    }

    private var recordSection: some View {
        Section {
            Button {
                Task { await model.record() }
            } label: {
                HStack {
                    Spacer()
                    Text("\(model.nextSetNo) セット目を記録").bold()
                    Spacer()
                }
            }
            .disabled(model.recording)

            if let remaining = model.restRemaining {
                LabeledContent("インターバル") {
                    Text(remaining).monospacedDigit()
                }
            }
        }
    }
}

#Preview {
    LogView()
}
