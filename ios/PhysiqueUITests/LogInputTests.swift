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
        XCTAssertTrue(tab.waitForExistence(timeout: UITimeout.slow), "筋トレタブが出ること")
        tab.tap()
    }


    /// 「種目を足す」からルーティンの n 番目を足す（#336）。
    ///
    /// **空から始まる**ので、入力まわりを見るテストは先にこれを呼ぶ
    private func addFromRoutine(_ index: Int) {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: UITimeout.slow), "種目を足すが出ること")
        XCTAssertTrue(add.isHittable, "押せる位置にあること")
        add.tap()

        let rows = app.buttons.matching(identifier: "pickExercise")
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: UITimeout.normal), "一覧が開くこと")
        let row = rows.element(boundBy: index)
        XCTAssertTrue(row.isHittable, "ルーティンの種目が押せる位置にあること")
        row.tap()

        // **シートが閉じきるまで待つ。** 続けて呼ぶと、閉じかけのシートの上で
        // 「種目を足す」を押すことになり、2回目が効かない（CI でだけ出た）
        XCTAssertTrue(
            app.navigationBars["種目を足す"].waitForNonExistence(timeout: UITimeout.normal),
            "シートが閉じること"
        )
    }

    /// #336。**ルーティンは並べず、選ぶときに一番上に出す。**
    /// 実際にはやらない種目が並ぶと、その日やったものが埋もれる
    func test_ルーティンは並ばず選ぶときに上に出る() {
        XCTAssertFalse(
            app.buttons["routineRow"].waitForExistence(timeout: 2),
            "最初は何も並んでいないこと"
        )

        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: UITimeout.slow), "種目を足すが出ること")
        add.tap()

        XCTAssertTrue(
            app.staticTexts["今日のルーティン"].waitForExistence(timeout: UITimeout.normal),
            "一番上に今日のルーティンが出ること"
        )
    }

    func test_足した種目が並んで入力まで開ける() {
        addFromRoutine(0)

        let row = app.buttons["routineRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: UITimeout.slow), "足した種目が並ぶこと")
        XCTAssertTrue(row.isHittable, "押せる位置にあること")

        // **既定で開いている**（#337）。足すたびにタップして開くのは手間
        let weight = app.textFields["weightInput"].firstMatch
        XCTAssertTrue(weight.waitForExistence(timeout: UITimeout.normal), "最初から入力欄が開いていること")
        XCTAssertTrue(weight.isHittable, "重量が打てる位置にあること")
        XCTAssertTrue(app.textFields["repsInput"].firstMatch.isHittable, "レップが打てる位置にあること")
        XCTAssertTrue(app.textFields["rirInput"].firstMatch.isHittable, "RIR が打てる位置にあること")

        // **種目の数だけある**（#337）。識別子だけで引くと曖昧になる
        let add = app.buttons.matching(identifier: "addSet").firstMatch
        XCTAssertTrue(add.exists, "セットを足すが出ること")
        XCTAssertTrue(add.isHittable, "押せる位置にあること")

        // **押したら閉じる**（トグル）。開いているものを閉じられること
        // **`app.textFields["x"]` は要素1つ。** 件数は matching で数える
        let allWeights = app.textFields.matching(identifier: "weightInput")
        let before = allWeights.count
        row.tap()
        XCTAssertLessThan(allWeights.count, before, "押すとその種目の入力欄が閉じること")
    }

    /// #337。**1つ開くと他が閉じる作りをやめた。**
    /// ジムではスーパーセットや種目間の確認で行き来する
    func test_複数の種目の入力欄が同時に開いている() {
        // **足しても一覧からは消えない。** 別の index を選ぶ
        addFromRoutine(0)
        addFromRoutine(1)

        let rows = app.buttons.matching(identifier: "routineRow")
        XCTAssertTrue(rows.firstMatch.waitForExistence(timeout: UITimeout.slow), "種目が出ること")
        XCTAssertGreaterThan(rows.count, 1, "2つ以上の種目が並んでいること")

        // 既定で全部開いているので、入力欄も種目の数だけある
        let weights = app.textFields.matching(identifier: "weightInput")
        XCTAssertTrue(weights.firstMatch.waitForExistence(timeout: UITimeout.normal))
        XCTAssertGreaterThan(weights.count, 1, "2つ以上の入力欄が同時に出ていること")

        // **どちらも押せること。** 出ているだけでは足りない（#202）
        XCTAssertTrue(weights.element(boundBy: 0).isHittable, "1つ目が押せること")
        XCTAssertTrue(weights.element(boundBy: 1).isHittable, "2つ目が押せること")
    }

    func test_種目を足す場所からカテゴリ別に選べる() {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: UITimeout.slow), "種目を足すが出ること")
        XCTAssertTrue(add.isHittable, "押せる位置にあること")
        add.tap()

        XCTAssertTrue(
            app.navigationBars["種目を足す"].waitForExistence(timeout: UITimeout.normal),
            "一覧が開くこと"
        )
        // **見出しは6つの大分類**（#288）。13並ぶと目的の種目まで遠い
        XCTAssertTrue(app.staticTexts["胸"].exists, "部位で分かれていること")
        XCTAssertTrue(app.staticTexts["脚"].exists, "大分類で畳まれていること")
        // **細かい部位は行に残る。** 分析は13部位前提なので見えなくしない
        XCTAssertTrue(app.staticTexts["大腿四頭"].exists, "細かい部位が行に出ること")

        // **打って絞れる。** 49種目なら畳むよりこちらが速い
        let search = app.searchFields.firstMatch
        XCTAssertTrue(search.waitForExistence(timeout: UITimeout.normal), "検索が出ること")
        XCTAssertTrue(search.isHittable, "打てる位置にあること")
        search.tap()
        search.typeText("スクワット")

        XCTAssertTrue(app.buttons["スクワット"].waitForExistence(timeout: UITimeout.normal),
                      "絞れること")
        // **シートの裏に居ない種目で見る。** ベンチプレスは下のルーティンにも
        // 並んでいるので、消えたかどうかを判定できない
        XCTAssertFalse(app.buttons["レッグエクステンション"].exists, "絞り込みで消えること")
    }

    /// #232 で踏んだ事故。足しても画面に何も起きなかった
    func test_ルーティンに無い種目を足すと行が増えて入力まで開ける() {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: UITimeout.slow), "種目を足すが出ること")
        add.tap()

        // スタブのルーティンは胸と脚だけ。大腿四頭の種目はこの日の想定に無い
        let lat = app.buttons["レッグエクステンション"]
        XCTAssertTrue(lat.waitForExistence(timeout: UITimeout.normal), "一覧に出ること")
        XCTAssertTrue(lat.isHittable, "押せる位置にあること")
        lat.tap()

        // **押した結果まで見る**（#217 の反省）。行が増え、その下が開く。
        // 足した種目は末尾に入り、全種目が開いているので画面外から始まる（#337）
        let weights = app.textFields.matching(identifier: "weightInput")
        XCTAssertTrue(weights.firstMatch.waitForExistence(timeout: UITimeout.normal), "入力欄が出ること")
        let added = weights.element(boundBy: weights.count - 1)
        XCTAssertTrue(added.exists, "足した種目の入力欄があること")

        // **届くことまで見る**（#202）。スクロールして届かないなら使えない
        var tries = 0
        while !added.isHittable, tries < 6 {
            app.swipeUp()
            tries += 1
        }
        XCTAssertTrue(added.isHittable, "スクロールすれば打てる位置に来ること")
    }

    /// #335。**懸垂・ディップスは重さに入れる数字が無い。**
    /// 0 のままだとトン数も e1RM も 0 になる
    func test_自重を選べる() {
        // **空から始まる**（#336）。足した行はもう開いている（#337）
        addFromRoutine(0)

        let toggle = app.buttons["bodyweightToggle"].firstMatch
        XCTAssertTrue(toggle.waitForExistence(timeout: UITimeout.normal), "自重が出ること")
        XCTAssertTrue(toggle.isHittable, "押せる位置にあること")

        // **押した結果まで見る**（#217 の反省）。単位が「+kg」に変わる
        toggle.tap()
        XCTAssertTrue(
            app.staticTexts["+kg"].firstMatch.waitForExistence(timeout: UITimeout.normal),
            "自重にすると加重のぶんを入れる欄だと分かること"
        )
    }

    /// #256。**記録済みも入力中も同じ行**で、その場で直せるところまで。
    func test_セットが行で出てその場で直せる() {
        addFromRoutine(0)

        let row = app.buttons["routineRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: UITimeout.slow), "足した種目が並ぶこと")

        // **押さない。足した時点で開いている**（#337）。押すと閉じる。
        // 開いた時点で1行ある（前回値が入る）
        let weight = app.textFields["weightInput"].firstMatch
        XCTAssertTrue(weight.waitForExistence(timeout: UITimeout.normal), "入力行が出ること")
        XCTAssertTrue(weight.isHittable, "打てる位置にあること")
        weight.tap()

        // **キーボードの ↑↓ で欄を移動できる**（食事と同じ・#229）
        let next = app.buttons["keyboardNext"]
        XCTAssertTrue(next.waitForExistence(timeout: UITimeout.normal), "次へが出ること")
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

    /// #264。**マスタに無い種目をその場で登録して、すぐ打てる**ところまで。
    func test_種目をマスタに登録してそのまま使える() {
        let add = app.buttons["addExercise"]
        XCTAssertTrue(add.waitForExistence(timeout: UITimeout.slow), "種目を足すが出ること")
        add.tap()

        let register = app.buttons["openExerciseRegister"]
        XCTAssertTrue(register.waitForExistence(timeout: UITimeout.normal), "新規が出ること")
        XCTAssertTrue(register.isHittable, "押せる位置にあること")
        register.tap()

        // **シートの中からシートを開く。** XCUITest では遅い環境で開き切る前に
        // 見てしまうことがあるので、まず navigation bar の出現を待つ
        XCTAssertTrue(
            app.navigationBars["種目を登録"].waitForExistence(timeout: UITimeout.slow),
            "登録画面が開くこと"
        )

        let name = app.textFields["newExerciseName"]
        XCTAssertTrue(name.waitForExistence(timeout: UITimeout.slow), "名前の欄が出ること")
        XCTAssertTrue(name.isHittable, "打てる位置にあること")
        name.tap()
        name.typeText("ケーブルクロスオーバー")

        // **キーボードに隠れない位置にあること**（#202）
        let save = app.buttons["saveExercise"]
        XCTAssertTrue(save.isHittable, "登録が押せること")
        save.tap()

        // **シートが2枚閉じる。** 閉じ終わるのを待たずに見ると、
        // 遅い環境（CI）で過渡状態を踏む
        let sheet = app.navigationBars["種目を足す"]
        XCTAssertTrue(
            sheet.waitForNonExistence(timeout: UITimeout.slow),
            "一覧が閉じること"
        )

        // **登録したらそのまま打てる**（行が増えて開く）
        XCTAssertTrue(
            app.textFields["weightInput"].firstMatch.waitForExistence(timeout: UITimeout.slow),
            "登録した種目で入力が開くこと"
        )
    }

    func test_Dayを手でずらせる() {
        let change = app.buttons["changeDay"]
        XCTAssertTrue(change.waitForExistence(timeout: UITimeout.slow), "変更が出ること")
        XCTAssertTrue(change.isHittable, "押せる位置にあること")
        change.tap()

        XCTAssertTrue(
            app.navigationBars["今日やる日"].waitForExistence(timeout: UITimeout.normal),
            "選ぶ画面が開くこと"
        )
        let day2 = app.buttons["2日目  脚"]
        XCTAssertTrue(day2.waitForExistence(timeout: UITimeout.normal), "別の Day が出ること")
        XCTAssertTrue(day2.isHittable, "押せる位置にあること")
        day2.tap()

        // **押した結果まで見る**（#217 の反省）
        XCTAssertTrue(
            app.staticTexts["2 / 2日目  脚"].waitForExistence(timeout: UITimeout.normal),
            "選んだ Day に切り替わること"
        )
    }

    func test_前の日に移動できる() {
        // 食事と同じ形（#193）
        let back = app.buttons.matching(identifier: "chevron.left").firstMatch
        XCTAssertTrue(back.waitForExistence(timeout: UITimeout.slow), "戻るが出ること")
        XCTAssertTrue(back.isHittable, "押せる位置にあること")
    }
}
