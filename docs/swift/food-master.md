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
