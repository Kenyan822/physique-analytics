import SwiftUI

/// ジムでの入力画面（要件 T-01〜T-04）。
///
/// **片手・手袋・汗の状態で使える**ことが最優先。数値はキーボードではなく
/// ステッパーで刻み、RIR は1タップで選ぶ。
struct LogView: View {
    @State private var model = LogModel()

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
            .navigationTitle("記録 \(JST.displayString(from: model.date))")
            .task { await model.load() }
            .onChange(of: model.selectedExerciseId) { _, id in
                Task { await model.selectExercise(id) }
            }
            .alert("エラー", isPresented: $model.showError) {
                Button("閉じる", role: .cancel) {}
            } message: {
                Text(model.errorMessage)
            }
        }
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
