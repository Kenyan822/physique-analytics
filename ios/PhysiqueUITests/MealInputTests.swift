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

    func test_自動計算に戻せる() {
        // **Form の中の既定スタイルの Button は action が呼ばれなかった**（#217）。
        // 「引数を足す」と同じ形のボタンがここにもある
        openTargetSheet()
        type("180", into: "targetP")
        type("70", into: "targetF")
        type("250", into: "targetC")
        app.buttons["保存"].tap()

        XCTAssertTrue(app.staticTexts["180g"].waitForExistence(timeout: 5))

        openTargetSheet()
        let clear = app.buttons["clearManualTarget"]
        XCTAssertTrue(clear.waitForExistence(timeout: 5), "自動計算に戻すが出ること")
        XCTAssertTrue(clear.isHittable, "押せる位置にあること")
        clear.tap()

        XCTAssertTrue(
            app.staticTexts["180g"].waitForNonExistence(timeout: 5),
            "押すと手動の目標が消えること"
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

        // **押して効くところまで見る。** Form の中のボタンは
        // キーボードを閉じる .onTapGesture にタップを奪われていた（#217）
        record.tap()

        XCTAssertTrue(
            app.staticTexts["30g"].waitForExistence(timeout: 5),
            "Form の中の記録ボタンが本当に効くこと"
        )
    }

    // MARK: - 食品マスタ（#209）

    func test_マスタから選ぶ場所が押せる() {
        // **操作の入口が増えたので1本足す**（#202 の運用）
        let pick = app.buttons["pickFromMaster"]
        XCTAssertTrue(pick.waitForExistence(timeout: 20), "マスタから選ぶが出ること")
        XCTAssertTrue(pick.isHittable, "押せる位置にあること")
        pick.tap()

        XCTAssertTrue(
            app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 5),
            "一覧が開くこと"
        )
    }

    func test_マスタに登録する画面が開く() {
        // **親のシートを閉じると子が出ない**、を捕まえる。
        // 実機で「登録を押すと勝手に閉じる」として出た（#209）
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))

        app.buttons["openFoodRegister"].tap()

        XCTAssertTrue(
            app.navigationBars["マスタに登録"].waitForExistence(timeout: 5),
            "登録画面が開くこと（一覧が閉じてしまわないこと）"
        )
    }

    func test_引数を足す場所が押せる() {
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))
        app.buttons["openFoodRegister"].tap()
        XCTAssertTrue(app.navigationBars["マスタに登録"].waitForExistence(timeout: 5))

        // **スクロールしない。** 以前は押せないときに swipeUp() で広げてから
        // 探していたので、実機の「押せない」を緑のまま見逃した（#217）。
        // Form は見えていない行を作らないので、隠れていれば exists も false
        let add = app.buttons["addFoodComponent"]

        XCTAssertTrue(add.waitForExistence(timeout: 5), "開いた直後に引数を足すが出ること")
        XCTAssertTrue(add.isHittable, "スクロールせずに押せること")

        // **押して効くところまで見る。** isHittable だけでは
        // 「押すと画面ごと閉じる」を拾えなかった（#217）
        add.tap()

        XCTAssertTrue(
            app.navigationBars["マスタに登録"].exists,
            "押しても画面が閉じないこと"
        )

        XCTAssertTrue(
            app.textFields["componentName"].waitForExistence(timeout: 5),
            "押すと引数の入力欄が増えること"
        )
    }

    func test_全量が量に比例するを付けられる() {
        // **チェックを入れると基準量の欄が出る**（#218）。
        // 出ないと比例させる設定に辿り着けない
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))
        app.buttons["openFoodRegister"].tap()
        XCTAssertTrue(app.navigationBars["マスタに登録"].waitForExistence(timeout: 5))

        let toggle = app.switches["scalesWithAmount"]
        XCTAssertTrue(toggle.waitForExistence(timeout: 5), "チェックが出ること")
        XCTAssertTrue(toggle.isHittable, "押せる位置にあること")
        // **行の中央を押しても切り替わらない。** Form の Toggle は
        // スイッチ本体だけが反応する（iOS の標準の挙動）。
        // `tap()` は要素の中央＝ラベルの上を押してしまう
        toggle.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap()

        XCTAssertEqual(toggle.value as? String, "1", "チェックが入ること")

        XCTAssertTrue(
            app.textFields["baseAmount"].waitForExistence(timeout: 5),
            "入れると基準量の欄が出ること"
        )
    }

    func test_入力するときに量を変えられる() {
        // **ADR-0017 の狙いそのもの。** 登録した「いつもの量」を、
        // 記録するときに変えられるか（#218 の確認）
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))
        app.buttons["openFoodRegister"].tap()
        XCTAssertTrue(app.navigationBars["マスタに登録"].waitForExistence(timeout: 5))

        // **上の欄から順に埋める。** 下の欄に入力するとフォームが
        // スクロールし、上の欄がナビゲーションバーの裏に入って押せなくなる
        let name = app.textFields["foodName"]
        XCTAssertTrue(name.waitForExistence(timeout: 5))
        name.tap()
        name.typeText("プロテイン")

        // **キーボードを閉じてからボタンを押す。** 出たままだと、
        // 1タップ目は閉じるだけになる（`dismissesKeyboardOnTap` の仕様）
        app.buttons["完了"].tap()
        XCTAssertTrue(app.keyboards.firstMatch.waitForNonExistence(timeout: 5),
                      "キーボードが閉じること")

        app.buttons["addFoodComponent"].tap()
        let comp = app.textFields["componentName"]
        XCTAssertTrue(comp.waitForExistence(timeout: 5), "引数の欄が増えること")
        comp.tap()
        comp.typeText("量")

        // ツールバーは Form の外なので、キーボードが出たままでも押せる
        app.buttons["saveFood"].tap()

        // 一覧に戻って選ぶ → 量を聞く画面が開く
        let row = app.buttons["foodRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 5), "登録したものが一覧に出ること")
        row.tap()

        let amount = app.textFields["componentAmount"]
        XCTAssertTrue(amount.waitForExistence(timeout: 5), "量の入力欄が出ること")
        XCTAssertTrue(amount.isHittable, "**その場で変えられること**")

        let confirm = app.buttons["confirmFoodPick"]
        XCTAssertTrue(confirm.waitForExistence(timeout: 5), "入れるが出ること")
        XCTAssertTrue(confirm.isHittable, "押せる位置にあること")
    }

    func test_登録したものをあとから直せる() {
        // **ADR-0017 の前提そのもの。** 「登録時は量が固定だと思っていたが、
        // 2回目に毎回違うと気づく」経路を通しで踏む（#211）
        registerFood(named: "ゆで卵")

        let row = app.buttons["foodRow"].firstMatch
        XCTAssertTrue(row.waitForExistence(timeout: 5), "登録したものが一覧に出ること")
        row.swipeLeft()

        let edit = app.buttons["editFood"]
        XCTAssertTrue(edit.waitForExistence(timeout: 5), "スワイプで直すが出ること")
        XCTAssertTrue(edit.isHittable, "押せる位置にあること")
        edit.tap()

        XCTAssertTrue(
            app.navigationBars["登録した内容を直す"].waitForExistence(timeout: 5),
            "編集画面が開くこと"
        )

        let add = app.buttons["addFoodComponent"]
        XCTAssertTrue(add.waitForExistence(timeout: 5), "引数を足すが出ること")
        add.tap()

        // **引数を足せることが ADR-0017 の前提。** 押した結果まで見る（#217）
        XCTAssertTrue(
            app.textFields["componentName"].waitForExistence(timeout: 5),
            "引数の入力欄が増えること"
        )

        let save = app.buttons["saveFood"]
        XCTAssertTrue(save.waitForExistence(timeout: 5), "保存が出ること")
        XCTAssertTrue(save.isHittable, "保存が押せる位置にあること")
    }

    func test_近い順に並べる場所が押せる() {
        // **開いた瞬間にダイアログを出さない**ことも同時に見る。
        // 出ていたら一覧のボタンが押せない（要件 N-08）
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))

        let nearby = app.buttons["enableNearby"]
        XCTAssertTrue(nearby.waitForExistence(timeout: 5), "近い順に並べるが出ること")
        XCTAssertTrue(nearby.isHittable, "押せる位置にあること")
        nearby.tap()

        XCTAssertTrue(
            app.staticTexts["この場所でよく食べるものが上に出ている"].waitForExistence(timeout: 5),
            "有効になったことが分かること"
        )
    }

    // MARK: - 日付

    func test_前の日に移動できる() {
        let back = app.buttons.matching(identifier: "chevron.left").firstMatch
        XCTAssertTrue(back.waitForExistence(timeout: 20))
        XCTAssertTrue(back.isHittable)
    }

    // MARK: - 補助

    /// マスタに1件登録して、一覧に戻ったところまで進める。
    private func registerFood(named name: String) {
        app.buttons["pickFromMaster"].tap()
        XCTAssertTrue(app.navigationBars["マスタから選ぶ"].waitForExistence(timeout: 10))
        app.buttons["openFoodRegister"].tap()
        XCTAssertTrue(app.navigationBars["マスタに登録"].waitForExistence(timeout: 5))

        let field = app.textFields["foodName"]
        XCTAssertTrue(field.waitForExistence(timeout: 5), "名前の欄が出ること")
        field.tap()
        field.typeText(name)

        app.buttons["saveFood"].tap()
    }

    private func openTargetSheet() {
        // **識別子で引く。** 見出しは状態で変わる（目標 / 目標（手動））
        let target = app.buttons["openTarget"]
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
