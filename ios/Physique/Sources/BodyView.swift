import SwiftUI

/// 体組成の入力画面（要件 B-02 / B-03 / B-06）。
///
/// 記録画面と違って**測れた項目だけ入れる**。体脂肪率も周囲長も
/// 毎日測るものではないので、空欄をそのままにできることが要る。
struct BodyView: View {
    @State private var model: BodyModel
    /// 周囲長の「詳しく測る」を開いているか（#277）
    @State private var showingAllParts = false
    @State private var pickingDate = false

    init(api: APIClient, health: HealthSource? = nil, date: String = JST.dateString()) {
        _model = State(initialValue: BodyModel(api: api, health: health, date: date))
    }

    var body: some View {
        NavigationStack {
            Form {
                healthSection
                compositionSection
                fatigueSection
                measurementSection
            }
            .navigationBarTitleDisplayMode(.inline)
            // **食事・記録と同じ形**（#193 / #232 と揃える・#275）
            .toolbar { ToolbarItem(placement: .principal) { dateNav } }
            .dismissesKeyboardOnTap()
            .keyboardDoneButton()
            // **.task(id:) で日付が変わるたびに読み直す**
            .task(id: model.date) { await model.load(date: model.date) }
            // **一度許可したら黙って取り込む**（#274）
            .task { await model.syncHealthIfGranted(today: model.date) }
        }
    }

    /// 中央上部に置き、左右で1日ずつ（食事・記録と同じ・#275）
    private var dateNav: some View {
        HStack(spacing: 8) {
            Button { Task { await model.goToPreviousDay() } } label: {
                Image(systemName: "chevron.left")
            }

            Button { pickingDate = true } label: {
                Text(model.dateLabel).font(.headline)
            }
            .buttonStyle(.plain)

            // **未来には進めない。** 測りようがない日を開いても意味が無い
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

    /// Apple Health からの取り込み（要件 B-01 / B-09）。
    ///
    /// **最初の1回だけ押してもらう。** 起動のたびに権限ダイアログが出るのは
    /// 邪魔なので、許可を通すまでは自動で走らせない。
    /// 一度通したあとは画面を開くたびに黙って取り込む（#274）。
    @ViewBuilder
    private var healthSection: some View {
        if !model.canSyncHealth, let message = model.message {
            // HealthKit が無い端末でも、保存の結果は出す
            Section { Text(message).font(.footnote).foregroundStyle(.secondary) }
        }

        if model.canSyncHealth {
            Section {
                Button(label(for: model)) {
                    Task { await model.syncHealth(today: model.date) }
                }
                .disabled(model.isSaving)
                .accessibilityIdentifier("syncHealth")

                // **押したボタンのすぐ下に出す。** 画面の一番下に出していたので、
                // 押しても何も起きないように見えていた
                if let message = model.message {
                    Text(message).font(.footnote).foregroundStyle(.secondary)
                        .accessibilityIdentifier("bodyMessage")
                }
            } footer: {
                Text(model.healthGranted
                     ? "開くたびに自動で取り込む。押すと今すぐ取り込む"
                     : "体重・体脂肪率・歩数・睡眠。Watch があれば HRV・安静時心拍・深睡眠も")
            }
        }
    }

    private func partRow(_ part: BodyPart) -> some View {
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

    private func label(for model: BodyModel) -> String {
        if model.isSaving { return "取り込み中…" }

        return model.healthGranted ? "今すぐ取り込む" : "Apple Health から取り込む"
    }

    private var compositionSection: some View {
        Section("体組成") {
            NumberRow(
                label: "体重", unit: "kg", value: $model.weightKg,
                previous: model.previousWeightKg, max: 300
            )
            NumberRow(label: "体脂肪率", unit: "%", value: $model.bodyfatPct, previous: nil, max: 70)

            Button(model.isSaving ? "保存中…" : "体組成を保存") {
                Task { await model.saveDaily(date: model.date) }
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
            // **既定は首とウエストだけ**（#277）。海軍式の体脂肪率推定に要る2つ。
            // 残りは見た目を追うためのもので、毎回は測らない
            ForEach(BodyPart.essentials, id: \.self) { partRow($0) }

            if showingAllParts {
                ForEach(BodyPart.optionals, id: \.self) { partRow($0) }
            } else {
                Button("詳しく測る") { showingAllParts = true }
                    .font(.callout)
                    .accessibilityIdentifier("showAllParts")
            }

            Button(model.isSaving ? "保存中…" : "周囲長を保存") {
                Task { await model.saveMeasurement(date: model.date) }
            }
            .disabled(model.isSaving || model.parts.isEmpty)
        } header: {
            Text("周囲長")
        } footer: {
            Text(model.previousMeasurement.map { "前回 \($0.date)" }
                 ?? "起床直後・食事前に測る。体組成計の体脂肪率は水分量でぶれるので、"
                 + "月1で首とウエストを測ると答え合わせになる")
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
