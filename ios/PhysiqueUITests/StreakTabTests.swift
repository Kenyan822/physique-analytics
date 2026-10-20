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

    /// #276。**カレンダーから中身に降りられる**
    func test_日をタップするとその日の中身が出る() {
        let tab = app.tabBars.buttons["継続"]
        XCTAssertTrue(tab.waitForExistence(timeout: UITimeout.slow), "継続タブが出ること")
        tab.tap()

        let day = app.buttons["streakDay"].firstMatch
        XCTAssertTrue(day.waitForExistence(timeout: UITimeout.normal), "日のマスが出ること")
        XCTAssertTrue(day.isHittable, "押せる位置にあること")
        day.tap()

        // 食事・筋トレ・体組成が1画面に出る
        XCTAssertTrue(app.staticTexts["食事"].waitForExistence(timeout: UITimeout.normal),
                      "食事が出ること")
        // **未達の理由が出る。** 「✗」だけでは何を直せばいいか分からない
        XCTAssertTrue(app.staticTexts["C が 50g 足りない"].exists, "未達の理由が出ること")

        // シートは medium で開くので、下の方はスクロールして見る
        XCTAssertTrue(app.staticTexts["筋トレ"].exists, "筋トレが出ること")
        app.swipeUp()
        XCTAssertTrue(app.staticTexts["体組成"].waitForExistence(timeout: UITimeout.normal),
                      "体組成が出ること")

        let close = app.buttons["閉じる"]
        XCTAssertTrue(close.isHittable, "閉じるが押せること")
    }
}
