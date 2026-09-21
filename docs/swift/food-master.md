# 食品マスタ —— 引数つきの計算式

[ADR-0017](../adr/0017-food-master-with-parameters.md) の実装。
**同じ計算を Go と Swift の両方が持っている**ので、そこが要点。

## なぜ両方が持つのか

| | 何のために計算するか |
|---|---|
| Swift | **打ちながら見せる。** 量を変えた瞬間に PFC を出す |
| Go | **保存する値を決める。** クライアントを信用しない |

サーバだけで計算すると、量を変えるたびに往復が要る。ジムや外出先で
使うものなので、そこで待たせたくない。

**食い違うと、画面に出ていた値が記録した瞬間に変わる。**

## 一致をテストで固定している

```swift
@Suite("サーバと同じ計算になる")
struct FoodItemParityTests {
    @Test("プロテイン 45g")
    func protein45() { assertSame(protein.expand(["量": 45]), 36.0, 2.25, 3.0) }
```

期待値は `internal/foodmaster.Expand` に同じ入力を与えて出したもの。
**片方だけ式を変えたら、ここが落ちる。**

Go 側にも同じケースがある（`internal/foodmaster/foodmaster_test.go`）。
数字を2か所に書いているので重複に見えるが、**重複していることに意味がある** ——
片方を直したときに、もう片方が追随していないと分かる。

## 計算

```
引数が無い  →  登録した PFC をそのまま返す
引数がある  →  Σ（基準量あたりの PFC × 入力量 / 基準量）
```

```swift
func expand(_ amounts: [String: Double]) -> Macros {
    guard hasComponents else {
        return Macros(..., proteinG: proteinG ?? 0, ...)
    }

    var p = 0.0
    for comp in components {
        guard comp.basisAmount > 0 else { continue }
        let ratio = (amounts[comp.name] ?? comp.defaultAmount) / comp.basisAmount
        p += comp.proteinG * ratio
    }
    ...
}
```

### 渡さなかった引数は既定値、0 は 0

```swift
amounts[comp.name] ?? comp.defaultAmount
```

**`?? ` が効くのは「キーが無い」ときだけ。** `0` を渡せば 0 として扱われる。
「今日は砂糖を入れなかった」を表せる。

`amounts[comp.name] ?? 0` にすると、触っていない引数が全部 0 になる。

### 基準量 0 を飛ばす

DB の `check (basis_amount > 0)` で防いでいるが、**割り算の前に落ちない**
ことをコード側でも保証する。古い端末から来た値で `inf` を出さない。

## 数値は文字列のまま持つ

```swift
struct FoodComponentDraft {
    var basisAmount = "100"
    var proteinG = ""
```

数値に直しながら持つと「30.」で丸められて小数が打てない
（[MealDraft と同じ罠](../../web/app/meals/MealForm.tsx)を iOS でも踏んだ）。

## 引数が無いときは空配列を返す

サーバ側で `null` ではなく `[]` に揃えている。

```go
items[i].Components = emptyIfNil(byItem[items[i].Id])
```

**引数なしの方が多い**（ADR-0017 の既定の経路）ので、ここで `null` が来ると
`components.isEmpty` の前に `?` が要る場所が増える。

## シートの入れ子に注意

```swift
.sheet(isPresented: $showingFoodList) { foodListSheet }

private var foodListSheet: some View {
    NavigationStack { ... }
        .sheet(isPresented: $registeringFood) { foodRegisterSheet }
}
```

**親を閉じると子も出ない。** 「登録」で一覧を閉じてから登録画面を開こうとして、
何も出ない状態になった（実機で「登録ボタンを押すと勝手に閉じる」として出た）。

一覧は開いたままにして、上に重ねる。登録後はその一覧に戻り、足したものが並ぶ。

UI テストで固定してある。

```swift
func test_マスタに登録する画面が開く() {
    app.buttons["openFoodRegister"].tap()
    XCTAssertTrue(app.navigationBars["マスタに登録"].waitForExistence(timeout: 5),
                  "登録画面が開くこと（一覧が閉じてしまわないこと）")
}
```

**バグを戻して落ちることも確認した**（[testing.md](testing.md#テストが機能するかを確かめる)）。

## `Form` は見えていない行を作らない

登録画面に引数のセクションを足したら、UI テストが落ちた。

```
XCTAssertTrue failed - 引数を足すが出ること
```

`.presentationDetents([.medium])` の高さでは下のセクションが描画されず、
**`exists` すら false になる。** 「画面外にあるが存在する」ではなく
「存在しない」。

| | 何を見るか |
|---|---|
| `exists` | 要素が**アクセシビリティツリーにある**か |
| `isHittable` | その座標を押したら届くか |

**`List` / `Form` は行を遅延生成する**ので、`exists` も当てにならない。
スクロールして初めて生える。

対処は2つ入れた。

- **シートの既定を `.large` にする。** 項目が増えたので medium では下が隠れる
- テスト側でも、見つからなければ一度スワイプしてから探す

```swift
let add = app.buttons["addFoodComponent"]
if !add.waitForExistence(timeout: 3) { app.swipeUp() }
XCTAssertTrue(add.waitForExistence(timeout: 5), "引数を足すが出ること")
```

## 型の本体に `init` を書くと、既定の `init` が消える

登録済みの引数を編集欄に戻すために `FoodComponentDraft(component)` を足したら、
`FoodComponentDraft()`（引数を1つ増やすとき）がコンパイルできなくなった。

```swift
struct FoodComponentDraft {
    var name = ""
    var basisAmount = "100"

    init(_ c: FoodItemComponent) { ... }   // ← これを本体に書くと
}

FoodComponentDraft()   // error: Missing argument for parameter #1
```

**Swift は「独自の `init` を1つでも本体に書いたら、合成をやめる」。**
memberwise init も、全部に既定値がある型の `init()` も消える。

拡張に置けば両方残る。

```swift
extension FoodComponentDraft {
    init(_ c: FoodItemComponent) {
        self.init()          // 合成された init を呼べる
        name = c.name
    }
}
```

Python の `__init__` を足しても既定の構築が消えるだけ、TypeScript には
そもそも合成が無い、という感覚で書くとここで詰まる。
**「便利な init を足す」は常に拡張側**、と決めておくと踏まない。

## 登録と編集で同じ画面を使う

違いは送り先（`POST` か `PATCH`）だけなので、画面は1つにした。

```swift
private(set) var editingFoodID: UUID?     // nil なら新規

if let id = editingFoodID {
    _ = try await api.updateFoodItem(id: id, input)
} else {
    _ = try await api.createFoodItem(input)
}
```

画面側は題名とボタンの文言を出し分けるだけ。

```swift
.navigationTitle(model.editingFoodID == nil ? "マスタに登録" : "登録した内容を直す")
```

**分けると「引数を足す」を2か所に書くことになる。** 引数の編集は
この機能の要（ADR-0017）なので、そこが二重になるのは避けたい。

## 一覧の行で「選ぶ」と「直す」を両立させる

行のタップは**選ぶ**（入力が埋まる）のままにして、直す・消すは
`swipeActions` に寄せた。

```swift
Button { model.pickFood(item) } label: { foodRow(item) }
    .buttonStyle(.plain)
    .swipeActions(edge: .trailing) {
        Button(role: .destructive) { ... } label: { Label("消す", systemImage: "trash") }
        Button { ... } label: { Label("直す", systemImage: "pencil") }.tint(.blue)
    }
```

行に `i` ボタンを置く案もあったが、**選ぶときのタップ領域が狭くなる**。
この画面は片手で速く触るのが狙い（#188）なので、めったに使わない方を
スワイプに逃がした。記録の一覧が既にスワイプ削除なので操作も揃う。

XCUITest からは `cell.swipeLeft()` で開ける。

## 上書きで済ませるのは `usedCount` を残すため

消して作り直すと `used_count` が 0 に戻り、一覧の並び（よく使う順）から落ちる。
`PATCH` なら回数はそのまま。

## `presentationDetents` の引数は `Set` —— 書いた順は効かない

```swift
.presentationDetents([.large, .medium])   // ← large が既定にはならない
```

**`Set<PresentationDetent>` なので順序を持たない。** 省略時、SwiftUI は
**最小の detent** を選ぶ。`.large` を先頭に書いても `.medium` で開く。

#217 を追う途中で見つけた。`.medium` では引数のセクションが画面の下に落ち、
**`Form` は見えていない行を作らない**ので、そこにボタンは存在しない。

既定を決めたいときは `selection:` を使う。

```swift
@State private var detent: PresentationDetent = .large

.presentationDetents([.medium, .large], selection: $detent)
```

配列リテラルの見た目に引きずられる罠。`Set` を取る API はこれ以外にもあるので、
**「順番に意味がありそうなのに `Set`」を見たら `selection:` 相当を探す。**

> この画面自体は最終的に**シートをやめて push した**ので `presentationDetents`
> は使っていない（次節）。罠としては残るので書いておく。

### UI テストがこれを緑のまま通していた

```swift
let add = app.buttons["addFoodComponent"]
if !add.waitForExistence(timeout: 3) { app.swipeUp() }   // ← フォールバック
XCTAssertTrue(add.isHittable)
```

`swipeUp()` でシートが `.large` に広がってから探していたので通っていた。
**「最初から押せるか」を見ていなかった。**

**UI テストに「出なかったらスクロールする」を書かない。** 出ないこと自体が
症状なので、回避策を書いた時点でテストの意味が無くなる。

### `isHittable` だけでは足りない

「押せるのに何も起きない」形の不具合を拾えない。**押した結果まで見る。**

```swift
add.tap()

XCTAssertTrue(
    app.textFields["componentName"].waitForExistence(timeout: 5),
    "押すと引数の入力欄が増えること"
)
```

## 入れ子のシートは、中で状態を変えると剥がれる

「引数を足すが押せない」を追っている途中で見つけた、別の不具合（#217）。
**利用者の症状の直接の原因ではなかった**が、押すと画面ごと閉じるのは確かなので直した。

登録画面を一覧シートの**上にシートで重ねて**いた。

```swift
private var foodListSheet: some View {
    NavigationStack { List { ... } }
}
.sheet(isPresented: $registeringFood) { foodRegisterSheet }   // ← 入れ子
```

この状態で `model` を触ると、**両方のシートが閉じて食事の本画面に戻る。**

### 切り分け方

ボタンの中身を空にして比べると1回で分かる。

| ボタンの中身 | 結果 |
|---|---|
| 空（何もしない） | **シートは開いたまま** |
| `model.addFoodComponent()` | **両方閉じる** |

**タップは届いている。** 引き金は `@Observable` の状態変更で、
`MealView.body` が作り直されると、その中で組み立てている `foodListSheet` も
作り直され、そこにぶら下がっていた `.sheet` が外れる。

UI テストは `app.debugDescription` を撮ると早い。ツリーの頂点が
`マスタに登録` ではなく食事の本画面になっていれば、閉じたと確定できる。

### 直し方：重ねずに push する

一覧は既に `NavigationStack` を持っているので、その上に積めばよい。

```swift
NavigationStack {
    List { ... }
        .navigationDestination(item: $registeringFood) { _ in foodEditorScreen }
}
```

**経路を持つのは `NavigationStack`** なので、body が作り直されても消えない。
ついでに3つ片付く。

| | |
|---|---|
| 高さ | push した画面は全高。`presentationDetents` が要らなくなる |
| 戻る | 標準の戻るボタンが「やめる」の役目になる |
| #209 の件 | 「親を閉じると子が出ない」も構造ごと消える |

`.cancellationAction` の `ToolbarItem` は**足さない**。push した画面に置くと
戻るボタンが消える。

### シートを重ねたくなったら

**1つの `NavigationStack` に push できないか先に考える。** シートの入れ子は
「開くとき」「閉じるとき」「中で状態を変えたとき」の3方向で壊れる。
#209 と #217 はどちらも同じ構造から出た別の症状だった。

## `onTapGesture` は Form の中の Button からタップを奪う

**これが「引数を足すが押せない」の原因**（#217）。

キーボードを閉じるために、画面全体にこう付けていた。

```swift
content
    .scrollDismissesKeyboard(.interactively)
    .onTapGesture(perform: dismissKeyboard)   // ← これ
```

**既定のスタイルの Button は action が呼ばれなくなる。**
「押せるのに何も起きない」になる。

### 紛らわしいのは、効くボタンもあること

同じ画面の `pickFromMaster` は動いていた。違いはこれだけ。

```swift
Button { ... } label: { ... }
    .buttonStyle(.plain)
    .contentShape(Rectangle())   // ← 自前の当たり判定を持っている
```

**「同じ modifier の下で動くボタンがある」ので、modifier を疑いにくい。**

### 切り分け方

`@State` のカウンタをボタンに足して、見出しに出す。

```swift
@State private var taps = 0

Button { taps += 1; model.addFoodComponent() } label: { ... }
...
Text("引数 taps=\(taps) count=\(model.foodComponents.count)")
```

| 出力 | 意味 |
|---|---|
| `taps=0` | **action が呼ばれていない**（タップが届いていない） |
| `taps=1 count=0` | action は呼ばれた。モデルへの反映が失敗 |
| `taps=1 count=1` | 反映済み。描画側の問題 |

`taps` は `@State` なので再描画の有無も同時に分かる。
**「押せない」を3つに割れる**ので、これを最初にやると速い。

### 効かなかった直し方

**`simultaneousGesture` にしても奪う。**

```swift
.simultaneousGesture(TapGesture().onEnded { dismissKeyboard() })   // ← 直らない
```

一度これで直ったと思ったが、実際に効いていたのは同時に入れた
`buttonStyle` の明示の方だった。**`Toggle` が同じ症状のまま残っていて**
（#218）気づいた。

### 直し方 —— キーボードが出ているときだけ受ける

```swift
@State private var keyboardUp = false

content
    .scrollDismissesKeyboard(.interactively)
    .gesture(
        TapGesture().onEnded { dismissKeyboard() },
        including: keyboardUp ? .all : .subviews
    )
    .onReceive(NotificationCenter.default.publisher(
        for: UIResponder.keyboardWillShowNotification)) { _ in keyboardUp = true }
    .onReceive(NotificationCenter.default.publisher(
        for: UIResponder.keyboardWillHideNotification)) { _ in keyboardUp = false }
```

**キーボードが下りているときは `.subviews`** —— この gesture は働かず、
下のコントロールがそのまま受ける。閉じたいのはキーボードが出ているときだけ
なので、機能は失われない。

`if` で付け外しすると view の識別が変わるので、`including:` を切り替える。

**これが本当の直し方。** `buttonStyle` を1つずつ明示して回るのは、
新しいコントロールを足すたびに踏む地雷を残すだけだった
（外しても効くことをテストで確認済み）。

### Form の Toggle は行の中央を押しても切り替わらない

これは**不具合ではなく iOS の標準の挙動**。スイッチ本体だけが反応する。

XCUITest の `tap()` は要素の中央＝ラベルの上を押すので、切り替わらない。

```swift
toggle.coordinate(withNormalizedOffset: CGVector(dx: 0.95, dy: 0.5)).tap()
```

**`value` を見れば「押せていない」と分かる**（`"0"` / `"1"`）。
これを確かめずにコード側を疑うと、動いているものを直そうとして時間を溶かす。

## キーボードが出ているとき、ボタンの1タップ目は閉じるだけ

`dismissesKeyboardOnTap` を「キーボードが出ているときだけ受ける」形にした
結果の**仕様**（#218 で本人と確認して A を選んだ）。

| | キーボードが下りている | 出ている |
|---|---|---|
| ボタン・トグル | **1タップで効く** | 1タップ目は閉じるだけ → 2タップ目で効く |
| 画面の空白 | 何も起きない | 閉じる（これが要望） |

**PFC を打ってそのまま記録する経路は影響を受けない。** キーボードの上に
「記録」を置いてあるので（`keyboardFocusBar`）、そこは1タップのまま。

2タップになるのは「打ってから引数を足す」のような頻度の低い場面だけ。

### 貫通させる方法は見つかっていない

`simultaneousGesture` でも Form の中のコントロールからタップを奪う
（確認済み）。**「キーボードが出ているときだけ受ける」が今の妥協点。**

## UI テストは上の欄から順に埋める

下の欄に入力すると Form がスクロールし、**上の欄がナビゲーションバーの裏に入る。**

```
TextField, {{32.0, 64.0}, {338.0, 22.0}}, identifier: 'foodName'
NavigationBar, {{0.0, 78.0}, {402.0, 54.0}}       ← y 78〜132
```

`exists` は true、`tap()` も例外を出さない。**次の `typeText` が
「Neither element nor any descendant has keyboard focus」で落ちて初めて気づく。**

座標を読めば一発で分かるので、**フォーカスが当たらないときは frame を見る。**
「閉じた直後のタップが吸われている」と読み違えて時間を溶かした。
