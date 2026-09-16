# `@Observable` と、画面が描き直される仕組み

SwiftUI は「状態が変わったら描き直す」。**どうやって変化を知るのか**が本題。

## 3つの属性の役割

| 属性 | 付ける場所 | 何をするか |
|---|---|---|
| `@Observable` | モデルの `class` | プロパティの**読み書きを追跡可能にする** |
| `@State` | View の中 | View の再生成をまたいで**実体を持ち続ける** |
| `@Bindable` | View の中 | `@Observable` なクラスから `$` で `Binding` を作る |

## `@Observable` はマクロ

```swift
@Observable
@MainActor
final class MealModel {
    private(set) var meals: [Meal] = []
    var draft = MealDraft()
}
```

`@Observable` は**コンパイル時にコードを書き足すマクロ**。
おおよそ次のように展開される。

```swift
final class MealModel: Observable {
    private let _$observationRegistrar = ObservationRegistrar()

    private var _meals: [Meal] = []
    var meals: [Meal] {
        get { _$observationRegistrar.access(self, keyPath: \.meals); return _meals }
        set { _$observationRegistrar.withMutation(self, keyPath: \.meals) { _meals = newValue } }
    }
}
```

読むと `access`、書くと `withMutation`。**これが仕組みのすべて。**

Xcode で `@Observable` を右クリック → `Expand Macro` すると実物が見られる。

## 「読んだこと」が記録される

SwiftUI は `body` を評価するとき、その評価を追跡ブロックで包む。

```
body を評価する
  └─ model.totals.kcal を読む
        └─ access(\.meals) が呼ばれる
              └─ 「この body は meals に依存している」と記録
```

あとで `meals` に書き込むと `withMutation` が走り、
記録されていた `body` だけが無効化される。

### だから依存の宣言が要らない

```swift
// 描画で読んだものが、そのまま依存になる
if let message = model.targetsMessage {
    Text(message)
}
```

`targetsMessage` を読んだ `body` だけが、`targetsMessage` の変化で描き直される。
`meals` が変わっても、`meals` を読んでいない `body` は動かない。

**これが旧 `ObservableObject` との決定的な違い。** `@Published` は
`objectWillChange` を1本しか持たず、**どれか1つでも変わると
そのオブジェクトを見ている View が全部描き直された**。

| | 粒度 | 宣言 |
|---|---|---|
| `ObservableObject` + `@Published` | オブジェクト単位 | `@Published` を1つずつ付ける |
| `@Observable` | **プロパティ単位、かつ実際に読んだものだけ** | 不要 |

### 計算プロパティも追跡される

```swift
var totals: MealTotals { MealTotals(of: meals) }
var remaining: Macros? { target.map { totals.remaining(from: $0) } }
```

`totals` 自体は格納プロパティではないが、中で `meals` を読むので
`access(\.meals)` が走る。**`remaining` を読んだ View は `meals` と `target` の
両方に依存する**ことになる。何も書かずにそうなる。

## `@State` が持つのは「実体」

```swift
struct MealView: View {
    @State private var model: MealModel

    init(api: APIClient) {
        _model = State(initialValue: MealModel(api: api))
    }
}
```

**`View` は構造体で、描き直しのたびに作り直される。**
`init` も毎回走る。だから

```swift
// これだと描き直すたびに新しい MealModel ができて、入力が消える
private var model = MealModel(api: api)
```

`@State` は値を View の外（SwiftUI が管理する領域）に置き、
同じ位置の View には同じ実体を渡す。

### `_model = State(initialValue:)` の意味

`@State private var model` は、実際には `_model` という
`State<MealModel>` 型の変数に展開される。`init` で引数を使って初期化したいときは、
プロパティラッパの実体（`_` 付き）に直接代入するしかない。

```swift
@State private var model: MealModel        // ← _model: State<MealModel> が生える
_model = State(initialValue: MealModel(api: api))
```

**`model = ...` とは書けない。** `init` の時点では `State` の中身に
アクセスできないため。

## `@Bindable` は「双方向の口」を作る

```swift
struct LoginView: View {
    @Bindable var auth: AuthModel
```

`@Observable` なクラスは `$` を持たない。`@Bindable` を付けると `$auth.email` の
ような `Binding` が作れる。**実体は持たない**（`@State` と違い、所有しない）。

このアプリでは `LoginView` だけが `@Bindable`。他の画面は
`model.xxx` を読むだけか、`@State private var model` で所有している。

### `Binding` は「読み書きのペア」

```swift
TextField("メールアドレス", text: $email)
```

`$email` は `Binding<String>` で、中身は `get` と `set` のクロージャ2つ。
`TextField` はキー入力のたびに `set` を呼ぶ → `withMutation` → 再描画。

## `private(set)` を多用している理由

```swift
private(set) var meals: [Meal] = []
private(set) var errorMessage: String?
var draft = MealDraft()          // これだけ書ける
```

**View から書けるのは「入力欄の中身」だけ**にしている。
`meals` を View から書けるようにすると、API を通さない状態変化ができてしまい、
サーバと画面がずれる。

読む分には `private(set)` でも追跡される（`get` は公開のまま）。

## 落とし穴

### `body` の外で読んでも追跡されない

```swift
.task { await model.load() }      // ここで読んだ値は依存にならない
```

`task` / `onAppear` / ボタンのクロージャは `body` の**評価**ではないので、
その中で読んだプロパティは依存として記録されない。
読んだ値を変数に入れて後で使う、ということをしなければ問題にならない。

### 配列の要素を書き換えても追跡される

```swift
meals.append(created)
meals.removeAll { $0.id == meal.id }
```

`meals` は値型（`Array`）なので、要素を変えることは
**`meals` そのものへの代入**になる。`withMutation` が走る。

参照型の配列だとこうならない。このアプリのモデルはすべて `struct`。

### `@MainActor` がほぼ必須

`@Observable` は再描画を起こす。**再描画はメインスレッドでしか行えない。**
だからモデルには全部 `@MainActor` が付いている。詳細は
[concurrency.md](concurrency.md)。
