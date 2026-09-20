import XCTest

/// 配線の確認だけ。**アプリが起動するところまで**を見る。
///
/// ここが通れば、UI テストのターゲット・スキーム・CI の経路ができている。
/// 中身は SaveButtonTests 以降で足す。
final class SmokeTests: XCTestCase {
    func test_アプリが起動する() {
        let app = XCUIApplication()
        app.launch()

        // **ログイン画面で止まる。** 偽の API を挿すのは次の段階
        XCTAssertTrue(
            app.staticTexts["physique"].waitForExistence(timeout: 10),
            "起動してログイン画面が出ること"
        )
    }
}
