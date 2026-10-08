import XCTest

/// 筋トレ記録の「押せるか」（要件 T-01 / #232）。
///
/// **食事と同じ方針。** ロジックは `swift test` が見ているので、
/// ここで見るのは届くかだけ。**通しの経路上で `isHittable` を見る**（#227）。
final class LogInputTests: XCTestCase {
    private var app: XCUIApplication!

    override func setUp() {
        continueAfterFailure = false
        app = XCUIApplication()
        app.launchArguments = ["-uiTesting"]
        app.launch()
        // **タブは identifier を持たない。** `.tabItem { Label("記録", systemImage:) }`
        // のボタンは label が「記録」で、SF Symbol 名では引けない。
        // `app.buttons["dumbbell"]` はレイアウト途中の別要素に当たることがあり、
        // 通ったり落ちたりしていた
        let tab = app.tabBars.buttons["記録"]
        XCTAssertTrue(tab.waitForExistence(timeout: 30), "筋トレタブが出ること")
        tab.tap()
    }

    func test_今日の想定が並んで入力まで開ける() {
        // **最初から並んでいること。** 以前は別画面で50種目から選んでいた
        let row = app.buttons["routineRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20), "今日の想定が出ること")
        XCTAssertTrue(row.isHittable, "押せる位置にあること")
        row.tap()

        // **その行の下に開く**（#232）。押せて、打てるところまで見る
        let weight = app.textFields["weightInput"].firstMatch
        XCTAssertTrue(weight.waitForExistence(timeout: 5), "押すと入力欄が開くこと")
        XCTAssertTrue(weight.isHittable, "重量が打てる位置にあること")
        XCTAssertTrue(app.textFields["repsInput"].firstMatch.isHittable, "レップが打てる位置にあること")
        XCTAssertTrue(app.textFields["rirInput"].firstMatch.isHittable, "RIR が打てる位置にあること")

        let add = app.buttons["addSet"]
        XCTAssertTrue(add.exists, "セットを足すが出ること")
        XCTAssertTrue(add.isHittable, "押せる位置にあること")

        // **同じ行をもう一度押したら閉じる**（トグル）
        row.tap()
        XCTAssertFalse(
            app.textFields["weightInput"].firstMatch.waitForExistence(timeout: 2),
            "もう一度押すと閉じること"
        )
    }

    func test_種目を足す場所からカテゴリ別に選べる() {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: 20), "種目を足すが出ること")
        XCTAssertTrue(add.isHittable, "押せる位置にあること")
        add.tap()

        XCTAssertTrue(
            app.navigationBars["種目を足す"].waitForExistence(timeout: 5),
            "一覧が開くこと"
        )
        // **カテゴリ別。** ご要望の「カテゴリ別に全部開いている」はここ
        XCTAssertTrue(app.staticTexts["胸"].exists, "部位で分かれていること")
        XCTAssertTrue(app.staticTexts["大腿四頭"].exists, "別の部位も出ること")
    }

    /// #232 で踏んだ事故。足しても画面に何も起きなかった
    func test_ルーティンに無い種目を足すと行が増えて入力まで開ける() {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: 20), "種目を足すが出ること")
        add.tap()

        // スタブのルーティンは胸と脚だけ。大腿四頭の種目はこの日の想定に無い
        let lat = app.buttons["レッグエクステンション"]
        XCTAssertTrue(lat.waitForExistence(timeout: 5), "一覧に出ること")
        XCTAssertTrue(lat.isHittable, "押せる位置にあること")
        lat.tap()

        // **押した結果まで見る**（#217 の反省）。行が増え、その下が開く
        let weight = app.textFields["weightInput"]
        XCTAssertTrue(weight.waitForExistence(timeout: 5), "足すと入力欄が開くこと")
        XCTAssertTrue(weight.isHittable, "打てる位置にあること")
        XCTAssertTrue(app.buttons["addSet"].isHittable, "セットを足すが押せること")
    }

    /// #256。**記録済みも入力中も同じ行**で、その場で直せるところまで。
    func test_セットが行で出てその場で直せる() {
        let row = app.buttons["routineRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 20), "今日の想定が出ること")
        row.tap()

        // 開いた時点で1行ある（前回値が入る）
        let weight = app.textFields["weightInput"].firstMatch
        XCTAssertTrue(weight.waitForExistence(timeout: 10), "入力行が出ること")
        XCTAssertTrue(weight.isHittable, "打てる位置にあること")
        weight.tap()

        // **キーボードの ↑↓ で欄を移動できる**（食事と同じ・#229）
        let next = app.buttons["keyboardNext"]
        XCTAssertTrue(next.waitForExistence(timeout: 5), "次へが出ること")
        XCTAssertTrue(next.isHittable, "押せる位置にあること")
        next.tap()
        XCTAssertTrue(app.buttons["keyboardPrev"].isHittable, "前へも押せること")

        app.buttons["keyboardDone"].tap()

        // **＋でセットが増える**
        let before = app.textFields.matching(identifier: "weightInput").count
        let add = app.buttons["addSet"]
        XCTAssertTrue(add.isHittable, "セットを足すが押せること")
        add.tap()
        XCTAssertEqual(
            app.textFields.matching(identifier: "weightInput").count, before + 1,
            "行が1つ増えること"
        )
    }

    func test_Dayを手でずらせる() {
        let change = app.buttons["changeDay"]
        XCTAssertTrue(change.waitForExistence(timeout: 20), "変更が出ること")
        XCTAssertTrue(change.isHittable, "押せる位置にあること")
        change.tap()

        XCTAssertTrue(
            app.navigationBars["今日やる日"].waitForExistence(timeout: 5),
            "選ぶ画面が開くこと"
        )
        let day2 = app.buttons["2日目  脚"]
        XCTAssertTrue(day2.waitForExistence(timeout: 5), "別の Day が出ること")
        XCTAssertTrue(day2.isHittable, "押せる位置にあること")
        day2.tap()

        // **押した結果まで見る**（#217 の反省）
        XCTAssertTrue(
            app.staticTexts["2 / 2日目  脚"].waitForExistence(timeout: 5),
            "選んだ Day に切り替わること"
        )
    }

    func test_前の日に移動できる() {
        // 食事と同じ形（#193）
        let back = app.buttons.matching(identifier: "chevron.left").firstMatch
        XCTAssertTrue(back.waitForExistence(timeout: 20), "戻るが出ること")
        XCTAssertTrue(back.isHittable, "押せる位置にあること")
    }
}
