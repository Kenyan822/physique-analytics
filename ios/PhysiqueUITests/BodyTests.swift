import XCTest

/// 体組成の「押せるか」（#274 / #275 / #277）。
final class BodyTests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-uiTesting"]
        app.launch()

        // **タブは identifier を持たない。** label で引く（docs/swift/testing.md）
        let tab = app.tabBars.buttons["体組成"]
        XCTAssertTrue(tab.waitForExistence(timeout: UITimeout.slow), "体組成タブが出ること")
        tab.tap()
    }

    /// #275。食事・記録と同じ日付ナビ
    func test_日付を前後に移せる() {
        let back = app.buttons.matching(identifier: "chevron.left").firstMatch
        XCTAssertTrue(back.waitForExistence(timeout: UITimeout.normal), "戻るが出ること")
        XCTAssertTrue(back.isHittable, "押せる位置にあること")

        // **未来には進めない**
        let fwd = app.buttons.matching(identifier: "chevron.right").firstMatch
        XCTAssertFalse(fwd.isEnabled, "翌日に進めないこと")

        back.tap()
        XCTAssertTrue(fwd.isEnabled, "戻ったら進めること")
    }

    /// #277。既定は首とウエストだけ。残りは畳む
    func test_周囲長は既定で2つだけ出て開ける() {
        XCTAssertTrue(app.staticTexts["首"].waitForExistence(timeout: UITimeout.normal),
                      "首が出ること")
        XCTAssertTrue(app.staticTexts["ウエスト（へそ）"].exists, "ウエストが出ること")
        // 畳まれている
        XCTAssertFalse(app.staticTexts["上腕（右）"].exists, "既定では出ないこと")

        // **スクロールしてから見る。** 周囲長はフォームの一番下にある。
        // キーボードに隠れているわけではないので、届かないのは問題ではない
        let more = app.buttons["showAllParts"]
        XCTAssertTrue(more.exists, "詳しく測るがあること")
        app.swipeUp()
        XCTAssertTrue(more.isHittable, "スクロールすれば押せること")
        more.tap()

        XCTAssertTrue(app.staticTexts["上腕（右）"].waitForExistence(timeout: UITimeout.normal),
                      "開くと出ること")
    }
}
