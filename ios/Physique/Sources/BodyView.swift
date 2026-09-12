import SwiftUI

/// 体組成の入力画面（要件 B-02 / B-03 / B-06）。
///
/// 記録画面と違って**測れた項目だけ入れる**。体脂肪率も周囲長も
/// 毎日測るものではないので、空欄をそのままにできることが要る。
struct BodyView: View {
    @State private var model: BodyModel
    private let date: String

    init(api: APIClient, date: String = JST.dateString()) {
        _model = State(initialValue: BodyModel(api: api))
        self.date = date
    }

    var body: some View {
        NavigationStack {
            Form {
                compositionSection
                fatigueSection
                measurementSection

                if let message = model.message {
                    Section {
                        Text(message).font(.footnote).foregroundStyle(.secondary)
                    }
                }
            }
            .navigationTitle("体組成 \(date)")
            .task { await model.load(date: date) }
        }
    }

    private var compositionSection: some View {
        Section("体組成") {
            NumberRow(
                label: "体重", unit: "kg", value: $model.weightKg,
                previous: model.previousWeightKg, max: 300
            )
            NumberRow(label: "体脂肪率", unit: "%", value: $model.bodyfatPct, previous: nil, max: 70)

            Button(model.isSaving ? "保存中…" : "体組成を保存") {
                Task { await model.saveDaily(date: date) }
            }
            .disabled(model.isSaving)
        }
    }

    /// 疲労度（要件 B-06）。**1タップで入る形にする。**
    /// 毎日聞かれるものなので、操作を挟むと入力されなくなる。
    private var fatigueSection: some View {
        Section {
            HStack {
                ForEach(1...5, id: \.self) { n in
                    Button {
                        model.setFatigue(n)
                    } label: {
                        Text("\(n)")
                            .frame(maxWidth: .infinity, minHeight: 44)
                    }
                    .buttonStyle(.bordered)
                    .tint(model.fatigue == n ? .accentColor : .secondary)
                }
            }
        } header: {
            Text("疲労度")
        } footer: {
            Text("1 = 元気 / 5 = 動けない")
        }
    }

    private var measurementSection: some View {
        Section {
            ForEach(BodyPart.allCases, id: \.self) { part in
                NumberRow(
                    label: part.label, unit: "cm",
                    value: Binding(
                        get: { model.parts[part] },
                        set: { model.parts[part] = $0 }
                    ),
                    previous: model.previousMeasurement?.value(for: part),
                    max: part.maxCm
                )
            }

            Button(model.isSaving ? "保存中…" : "周囲長を保存") {
                Task { await model.saveMeasurement(date: date) }
            }
            .disabled(model.isSaving || model.parts.isEmpty)
        } header: {
            Text("周囲長")
        } footer: {
            Text(model.previousMeasurement.map { "前回 \($0.date)" } ?? "起床直後・食事前に測る")
        }
    }
}

/// 数値1件の入力。前回値との差分をその場で出す（要件 B-03）。
private struct NumberRow: View {
    let label: String
    let unit: String
    @Binding var value: Double?
    let previous: Double?
    let max: Double

    @State private var text = ""

    var body: some View {
        HStack {
            VStack(alignment: .leading, spacing: 2) {
                Text(label)
                HStack(spacing: 6) {
                    Text(previous.map { "前回 \($0)\(unit)" } ?? "前回なし")
                    if let diff = formatDiff(current: value, previous: previous) {
                        Text(diff).foregroundStyle(Color.accentColor)
                    }
                }
                .font(.caption2)
                .foregroundStyle(.secondary)
            }

            Spacer()

            TextField(previous.map { "\($0)" } ?? "—", text: $text)
                .keyboardType(.decimalPad)
                .multilineTextAlignment(.trailing)
                .frame(width: 90)
                .onChange(of: text) { _, new in
                    // 空にしたら未入力に戻す。0 を入れたことにしない
                    value = new.trimmingCharacters(in: .whitespaces).isEmpty
                        ? nil
                        : parseNumber(new).map { Swift.min($0, max) }
                }
                .onAppear { text = value.map { "\($0)" } ?? "" }

            Text(unit).font(.caption).foregroundStyle(.secondary)
        }
    }
}
