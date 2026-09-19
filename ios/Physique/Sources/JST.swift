import Foundation

/// 日付は JST 固定（ADR-0013）。
///
/// **端末のタイムゾーンに依存させない。** 海外にいても「日本時間の今日」で
/// 記録が並ばないと、時系列が1日ずれる。
enum JST {
    /// 日本標準時。夏時間が無いので固定オフセットで足りる
    static let timeZone = TimeZone(secondsFromGMT: 9 * 3600)!

    static let calendar: Calendar = {
        var c = Calendar(identifier: .gregorian)
        c.timeZone = timeZone
        return c
    }()

    private static let formatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = calendar
        f.timeZone = timeZone
        // ユーザーの暦設定（和暦など）に引きずられないようにする
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "yyyy-MM-dd"
        return f
    }()

    /// JST における日付を `yyyy-MM-dd` で返す。
    static func dateString(from date: Date = Date()) -> String {
        formatter.string(from: date)
    }

    /// JST の時刻を `HH:mm` で返す。食事の記録時刻に使う。
    ///
    /// **区分（朝食/昼食/…）はここで決めない。** サーバが導出する。
    /// クライアントごとに境界がずれるのを避けるため（#191）
    static func timeString(from date: Date = Date()) -> String {
        timeFormatter.string(from: date)
    }

    private static let timeFormatter: DateFormatter = {
        let f = DateFormatter()
        f.calendar = calendar
        f.timeZone = timeZone
        f.locale = Locale(identifier: "en_US_POSIX")
        f.dateFormat = "HH:mm"
        return f
    }()

    /// `yyyy-MM-dd` を Date にする（その日の JST 0:00）。
    static func date(from string: String) -> Date? {
        formatter.date(from: string)
    }

    /// `HH:mm` を Date にする。**時刻を選ぶ UI に渡すための器**で、
    /// 日付部分に意味は無い（JST の基準日に載せているだけ）。
    ///
    /// 読めない値は nil。空欄＝「時刻を記録していない」と区別する
    static func time(from string: String) -> Date? {
        timeFormatter.date(from: string)
    }

    /// `yyyy-MM-dd` を n 日ずらす。
    ///
    /// **文字列のまま足さない。** 月末・年末をまたぐと壊れる。
    /// 日付は JST 固定なので、UTC の正午を基準にすれば夏時間も関係ない
    static func shift(_ dateString: String, days: Int) -> String {
        guard let d = date(from: dateString),
              let shifted = calendar.date(byAdding: .day, value: days, to: d) else {
            return dateString
        }

        return self.dateString(from: shifted)
    }

    /// `9/13(土)` のような表示にする。
    static func displayString(from dateString: String) -> String {
        guard let date = self.date(from: dateString) else { return dateString }

        let comps = calendar.dateComponents([.month, .day, .weekday], from: date)
        guard let m = comps.month, let d = comps.day, let wd = comps.weekday else {
            return dateString
        }
        let symbols = ["日", "月", "火", "水", "木", "金", "土"]

        return "\(m)/\(d)(\(symbols[wd - 1]))"
    }
}
