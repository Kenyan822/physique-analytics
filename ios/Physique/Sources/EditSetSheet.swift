import SwiftUI

/// 記録済みセットを直す（要件 T-01 / #247）。
///
/// **打ち間違いはその場で気づく。** 直せないと、あとで分析するときに
/// 嘘の値が残る。
struct EditSetSheet: View {
    let set: PendingSet
    let onSave: (Double, Int, Int?) async -> Void
    let onDelete: () async -> Void

    /// **数値は文字列のまま持つ。** 数値に直しながら持つと最後の1桁が消せない（#229）
    @State private var weightText: String
    @State private var repsText: String
    @State private var rir: Int?
    @State private var working = false
    @FocusState private var focused: Bool

    init(
        set: PendingSet,
        onSave: @escaping (Double, Int, Int?) async -> Void,
        onDelete: @escaping () async -> Void
    ) {
        self.set = set
        self.onSave = onSave
        self.onDelete = onDelete
        _weightText = State(initialValue: Self.text(set.weightKg))
        _repsText = State(initialValue: String(set.reps))
        _rir = State(initialValue: set.rir)
    }

    var body: some View {
        NavigationStack {
            Form {
                Section("\(set.setNo)セット目") {
                    LabeledContent("重量") {
                        HStack(spacing: 4) {
                            TextField("", text: $weightText)
                                .keyboardType(.decimalPad)
                                .multilineTextAlignment(.trailing)
                                .focused($focused)
                                .accessibilityIdentifier("editWeight")
                            Text("kg").foregroundStyle(.secondary)
                        }
                    }

                    LabeledContent("レップ") {
                        HStack(spacing: 4) {
                            TextField("", text: $repsText)
                                .keyboardType(.numberPad)
                                .multilineTextAlignment(.trailing)
                                .accessibilityIdentifier("editReps")
                            Text("回").foregroundStyle(.secondary)
                        }
                    }

                    Picker("RIR", selection: $rir) {
                        Text("—").tag(Int?.none)
                        ForEach(0...10, id: \.self) { n in Text("\(n)").tag(Int?.some(n)) }
                    }
                }

                Section {
                    Button("このセットを消す", role: .destructive) {
                        working = true
                        Task { await onDelete() }
                    }
                    .accessibilityIdentifier("deleteSet")
                }
            }
            .navigationTitle("記録を直す")
            .navigationBarTitleDisplayMode(.inline)
            .toolbar {
                ToolbarItem(placement: .confirmationAction) {
                    Button("保存") {
                        guard let w = Double(weightText.trimmingCharacters(in: .whitespaces)),
                              let r = Int(repsText.trimmingCharacters(in: .whitespaces)) else { return }
                        working = true
                        Task { await onSave(w, r, rir) }
                    }
                    .disabled(working || !isValid)
                    .accessibilityIdentifier("saveSet")
                }
            }
        }
        .presentationDetents([.medium])
    }

    /// 空欄のまま保存すると 0kg × 0回 が残る
    private var isValid: Bool {
        Double(weightText.trimmingCharacters(in: .whitespaces)) != nil
            && Int(repsText.trimmingCharacters(in: .whitespaces)) != nil
    }

    private static func text(_ v: Double) -> String {
        v == v.rounded() ? String(Int(v)) : String(v)
    }
}
