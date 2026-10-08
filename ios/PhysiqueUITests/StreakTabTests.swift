import XCTest

/// ストリークの「押せるか」（#249）。
///
/// **食事・記録と同じ方針。** 中身の判定は `swift test` と API のテストが見ている。
/// ここで見るのは届くかだけ。
final class StreakTabTests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-uiTesting"]
        app.launch()
    }

    func test_継続タブを開いて月を戻せる() {
        // **タブは identifier を持たない。** label で引く（docs/swift/testing.md）
        let tab = app.tabBars.buttons["継続"]
        XCTAssertTrue(tab.waitForExistence(timeout: UITimeout.slow), "継続タブが出ること")
        XCTAssertTrue(tab.isHittable, "押せる位置にあること")
        tab.tap()

        XCTAssertTrue(
            app.otherElements["streakCalendar"].waitForExistence(timeout: UITimeout.normal),
            "カレンダーが出ること"
        )

        let prev = app.buttons["prevMonth"]
        XCTAssertTrue(prev.waitForExistence(timeout: UITimeout.normal), "前の月が出ること")
        XCTAssertTrue(prev.isHittable, "押せる位置にあること")

        // **未来には進めない**
        XCTAssertFalse(app.buttons["nextMonth"].isEnabled, "翌月に進めないこと")

        prev.tap()
        XCTAssertTrue(app.buttons["nextMonth"].isEnabled, "戻ったら翌月に進めること")
    }
}
