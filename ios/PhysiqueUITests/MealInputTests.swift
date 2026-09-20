import XCTest

/// **押せるか・届くか**を見る。ロジックは `swift test`（124件）が見ている。
///
/// 対象は実機でしか出なかった種類のバグだけ。1本15〜60秒かかるので広げない。
///
/// | 実際に踏んだもの | |
/// |---|---|
/// | キーボードが記録ボタンを隠す | **このテストが検出した**（#202） |
/// | 目標ボタンのタップ領域が文字だけ | #195 |
/// | シートの保存ボタンが画面外に落ちる | #195 |
final class MealInputTests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-uiTesting"]
        app.launch()
    }

    // MARK: - 目標

    func test_目標を保存できる() {
        openTargetSheet()

        type("180", into: "targetP")
        type("70", into: "targetF")
        type("250", into: "targetC")

        let save = app.buttons["保存"]
        XCTAssertTrue(save.waitForExistence(timeout: 5), "保存が出ること")
        XCTAssertTrue(save.isHittable, "保存が押せる位置にあること")
        save.tap()

        XCTAssertTrue(
            app.staticTexts["180g"].waitForExistence(timeout: 5),
            "保存した目標が画面に出ること"
        )
    }

    func test_目標を開く場所がタップできる() {
        let target = app.staticTexts["目標"]
        XCTAssertTrue(target.waitForExistence(timeout: 20))
        // 以前は caption サイズの文字だけがタップ領域だった（#195）
        XCTAssertTrue(target.isHittable)
    }

    // MARK: - 記録

    func test_PFCを入れて記録できる() {
        type("45", into: "draftP")
        type("5", into: "draftF")
        type("0", into: "draftC")

        // **キーボードが出たまま押せること。** Form の中のボタンはキーボードの
        // 下に隠れるので、バーに逃がしてある（#202 がこれを検出した）
        let record = app.buttons["keyboardPrimaryAction"]
        XCTAssertTrue(record.waitForExistence(timeout: 5), "キーボードに記録が出ること")
        XCTAssertTrue(record.isHittable, "記録がキーボードに隠れていないこと")
        record.tap()

        XCTAssertTrue(
            app.staticTexts["45g"].waitForExistence(timeout: 5),
            "記録した PFC が一覧に出ること"
        )
    }

    func test_キーボードを閉じれば記録ボタンも押せる() {
        type("30", into: "draftP")

        // 欄の外を押して閉じる
        app.staticTexts["今日の合計"].tap()

        let record = app.buttons["recordButton"]
        XCTAssertTrue(record.waitForExistence(timeout: 5))
        XCTAssertTrue(record.isHittable, "キーボードを閉じれば Form のボタンも押せること")
    }

    // MARK: - 日付

    func test_前の日に移動できる() {
        let back = app.buttons.matching(identifier: "chevron.left").firstMatch
        XCTAssertTrue(back.waitForExistence(timeout: 20))
        XCTAssertTrue(back.isHittable)
    }

    // MARK: - 補助

    private func openTargetSheet() {
        let target = app.staticTexts["目標"]
        XCTAssertTrue(target.waitForExistence(timeout: 20), "目標の行が出ること")
        target.tap()
        XCTAssertTrue(
            app.navigationBars["目標"].waitForExistence(timeout: 5),
            "目標のシートが開くこと"
        )
    }

    /// **識別子で引く。** タブバーにも「記録」があり、`Num` はラベルと
    /// TextField が別要素なので、名前や並び順では取れない
    private func type(_ value: String, into id: String) {
        let field = app.textFields[id]
        XCTAssertTrue(field.waitForExistence(timeout: 20), "\(id) が出ること")
        field.tap()
        field.typeText(value)
    }
}
