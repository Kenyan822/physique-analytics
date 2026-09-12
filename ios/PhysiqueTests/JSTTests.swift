import Foundation
import Testing

@testable import PhysiqueCore

@Suite("JST の日付")
struct JSTTests {
    /// 端末が UTC でも「日本時間の今日」で記録が並ばないと時系列が1日ずれる
    @Test("JST の深夜でも当日の日付になる")
    func lateNight() {
        // 2026-09-13 01:00 JST = 2026-09-12 16:00 UTC
        let d = Date(timeIntervalSince1970: 1_789_228_800)
        #expect(JST.dateString(from: d) == "2026-09-13")
    }

    @Test("JST の朝8時59分も当日")
    func earlyMorning() {
        // 2026-09-13 08:59 JST = 2026-09-12 23:59 UTC
        let d = Date(timeIntervalSince1970: 1_789_257_540)
        #expect(JST.dateString(from: d) == "2026-09-13")
    }

    @Test("往復できる")
    func roundTrip() {
        let s = "2026-09-13"
        let d = JST.date(from: s)
        #expect(d != nil)
        #expect(JST.dateString(from: d!) == s)
    }

    @Test("曜日つきで表示する", arguments: [
        ("2026-09-13", "9/13(日)"),
        ("2026-09-12", "9/12(土)"),
    ])
    func display(input: String, expected: String) {
        #expect(JST.displayString(from: input) == expected)
    }

    @Test("壊れた入力はそのまま返す")
    func broken() {
        #expect(JST.displayString(from: "なんか変") == "なんか変")
    }
}

extension JSTTests {
    /// JST の 23:59 が翌日にならないことも確かめる
    @Test("JST の 23:59 は当日のまま")
    func lateEvening() {
        // 2026-09-13 23:59 JST = 2026-09-13 14:59 UTC
        let d = Date(timeIntervalSince1970: 1_789_311_540)
        #expect(JST.dateString(from: d) == "2026-09-13")
    }
}
