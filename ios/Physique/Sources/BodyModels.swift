import Foundation

/// 周囲長の測定箇所（要件 B-02）。
///
/// 順序は `data/sample/measures.csv` と同じにする。検査票やCSVと
/// 見比べるときに並びが違うと読み違える。
enum BodyPart: String, CaseIterable, Sendable {
    case neck, shoulder, chest, waistNavel, hip, armR, thighR, calfR

    /// 画面に出す名前。
    var label: String {
        switch self {
        case .neck: "首"
        case .shoulder: "肩"
        case .chest: "胸"
        case .waistNavel: "ウエスト（へそ）"
        case .hip: "臀部"
        case .armR: "上腕（右）"
        case .thighR: "大腿（右）"
        case .calfR: "下腿（右）"
        }
    }

    /// 入力の上限（cm）。API の検証と合わせてある。
    var maxCm: Double {
        switch self {
        case .neck, .armR, .calfR: 100
        case .thighR: 150
        case .shoulder, .chest, .waistNavel, .hip: 200
        }
    }
}

/// 周囲長。`openapi.yaml` の BodyMeasurement と対応する。
struct BodyMeasurement: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let date: String
    var neckCm: Double?
    var shoulderCm: Double?
    var chestCm: Double?
    var waistNavelCm: Double?
    var hipCm: Double?
    var armRCm: Double?
    var thighRCm: Double?
    var calfRCm: Double?

    /// 測定箇所で引く。画面がキーで回せるようにする。
    func value(for part: BodyPart) -> Double? {
        switch part {
        case .neck: neckCm
        case .shoulder: shoulderCm
        case .chest: chestCm
        case .waistNavel: waistNavelCm
        case .hip: hipCm
        case .armR: armRCm
        case .thighR: thighRCm
        case .calfR: calfRCm
        }
    }
}

/// 周囲長の入力。**送らない項目は API 側で「変更しない」になる。**
struct BodyMeasurementInput: Codable, Sendable {
    let date: String
    var neckCm: Double?
    var shoulderCm: Double?
    var chestCm: Double?
    var waistNavelCm: Double?
    var hipCm: Double?
    var armRCm: Double?
    var thighRCm: Double?
    var calfRCm: Double?

    mutating func set(_ part: BodyPart, _ value: Double?) {
        switch part {
        case .neck: neckCm = value
        case .shoulder: shoulderCm = value
        case .chest: chestCm = value
        case .waistNavel: waistNavelCm = value
        case .hip: hipCm = value
        case .armR: armRCm = value
        case .thighR: thighRCm = value
        case .calfR: calfRCm = value
        }
    }
}

/// 日次記録。体重・体脂肪率・疲労度だけを扱う（要件 B-06 ほか）。
struct DailyMetrics: Codable, Identifiable, Hashable, Sendable {
    let id: UUID
    let date: String
    var weightKg: Double?
    var bodyfatPct: Double?
    var fatigue: Int?
}

/// 日次記録の入力。nil は「変更しない」。
struct DailyMetricsInput: Codable, Sendable {
    let date: String
    var weightKg: Double?
    var bodyfatPct: Double?
    var fatigue: Int?
}

/// 前回値との差分を表示用の文字列にする（要件 B-03）。
///
/// 差が無い・前回が無い場合は nil。**測っただけで変化が見えないと、
/// 続ける意味が分からなくなる**ので、その場で差を出す。
func formatDiff(current: Double?, previous: Double?) -> String? {
    guard let current, let previous else { return nil }

    // 0.1 の差が 0.10000000000000142 になるので丸める
    let diff = (current - previous).rounded(toPlaces: 2)
    guard diff != 0 else { return nil }

    return diff > 0 ? "+\(trim(diff))" : "−\(trim(abs(diff)))"
}

/// 入力欄の文字列を数値にする。読めなければ nil。
///
/// 全角を正規化するのは、日本語入力のまま打つと全角数字になるため。
/// 弾くと「打てない」に見える。
func parseNumber(_ text: String) -> Double? {
    let normalized = text
        .applyingTransform(.fullwidthToHalfwidth, reverse: false)?
        .trimmingCharacters(in: .whitespaces) ?? ""

    guard !normalized.isEmpty,
          normalized.allSatisfy({ $0.isNumber || $0 == "." }),
          let v = Double(normalized)
    else { return nil }

    return v
}

private func trim(_ v: Double) -> String {
    v == v.rounded() ? String(Int(v)) : String(v)
}

extension Double {
    func rounded(toPlaces places: Int) -> Double {
        let f = pow(10.0, Double(places))

        return (self * f).rounded() / f
    }
}
