# Swift Concurrency —— コンパイラがデータ競合を落とす

Swift 6 は**データ競合をコンパイルエラーにする**。Go の `-race`（実行時検出）と
根本的に違い、実行する前に落ちる。

`swift-tools-version: 6.2` なので、このプロジェクトは厳格モードで動いている。

## 3つの道具

| | 何を保証するか |
|---|---|
| `actor` / `@MainActor` | **同時に1つの実行文脈からしか触れない** |
| `Sendable` | 実行文脈をまたいで**渡してよい** |
| `async` / `await` | 待つ間、スレッドを手放す |

## `@MainActor` は「この型は常にメインスレッド」

```swift
@Observable
@MainActor
final class MealModel {
    private(set) var meals: [Meal] = []

    func load() async {
        meals = try await api.listMeals(from: date, to: date)
    }
}
```

型に付けると、**全プロパティと全メソッドがメインアクター隔離**になる。

- 外から呼ぶには `await` が要る（別アクターからなら）
- 中では `meals` を同期的に触れる。ロックが要らない

### `await` の前後でスレッドが変わらない

```swift
func load() async {
    isWorking = true                             // メインスレッド
    meals = try await api.listMeals(...)         // ネットワークは別スレッド
    isWorking = false                            // メインスレッドに戻る
}
```

`api.listMeals` は `nonisolated`（アクターに属さない）なので、
`await` した瞬間にメインスレッドを手放す。**戻ってきたらまたメインスレッド。**

Go の goroutine と違い、**どのスレッドで動くかが型で決まる**。
`DispatchQueue.main.async` を書く必要が無い。

### `await` は中断点であって、排他の解除でもある

```swift
func record() async {
    guard !draft.isEmpty else { return }
    isWorking = true
    defer { isWorking = false }

    let created = try await api.createMeal(...)   // ← ここで他のコードが割り込める
    meals.append(created)
}
```

**`await` をまたぐと、その間に別のタスクが同じモデルを触りうる。**
メインアクターなので同時実行はしないが、**交互には走る**（再入）。

だから `isWorking` で二重実行を防いでいる。ボタン側でも

```swift
.disabled(model.isSaving)
```

と二重に塞ぐ。片方だけだと、連打で2件登録されうる。

## `Sendable` は「渡してよい」印

アクターをまたいで値を渡すとき、コンパイラが `Sendable` を要求する。

```swift
struct APIClient: Sendable {
    let baseURL: URL
    var tokenProvider: TokenProvider?
    var transport: HTTPTransport
}
```

- `struct` で、全プロパティが `Sendable` なら自動で満たす
- `HTTPTransport` を `protocol HTTPTransport: Sendable` にしてあるのはこのため

### `@unchecked Sendable` は「自分で守る」宣言

```swift
final class PendingQueue: @unchecked Sendable {
    private let store: PendingStore
    private let lock = NSLock()

    func push(_ item: PendingSet) {
        lock.lock()
        defer { lock.unlock() }
        ...
    }
}
```

`class` は可変状態を持つので自動では `Sendable` にならない。
`@unchecked` は**チェックを外す代わりに、自分でロックする**という契約。

`PendingQueue` をアクターにしなかったのは、`push` / `all` / `remove` が
**同期メソッドのまま使いたい**から。アクターにすると全部 `await` が要り、
`LogModel` の呼び出しが `await queue.push(item)` になる。

### クロージャの `@Sendable`

```swift
typealias TokenProvider = @Sendable () async throws -> String?
```

このクロージャは `APIClient` に保持され、別のスレッドから呼ばれうる。
`@Sendable` を付けると、**キャプチャする値もすべて `Sendable`** でなければ
コンパイルが通らなくなる。

固定トークンで作る初期化子で踏んだのがこれ。

```swift
init(baseURL: URL, token: String?, transport: HTTPTransport = URLSessionTransport()) {
    let provider: TokenProvider? = token.map { value in
        { @Sendable in value }        // ← @Sendable を書かないと型が合わない
    }
    ...
}
```

`{ value }` だけだと**非 Sendable なクロージャ**と推論され、
`TokenProvider` に代入できない。

## `[auth]` でキャプチャするのは循環参照対策ではない

```swift
private var api: APIClient {
    APIClient(
        baseURL: AppConfig.apiBaseURL,
        tokenProvider: { [auth] in try await auth.accessToken() }
    )
}
```

`[auth]` は**キャプチャリスト**。`self` 経由ではなく `auth` を直接捕まえる。

`MainTabs` は `struct` なので `self` を強参照しても循環しない。
書いている理由は、**`self` をキャプチャすると `struct` 全体が
クロージャの寿命まで生き残る**のを避けるため。

`auth` は `@MainActor` な class なので、`tokenProvider`（`@Sendable`）から
呼ぶには `await` が要る。それが `try await auth.accessToken()`。

## `Task` は「ここから非同期を始める」

SwiftUI のボタンのクロージャは同期関数なので、`await` を直接書けない。

```swift
Button("ログアウト", role: .destructive) {
    Task { await auth.signOut() }
}
```

`Task { }` の中は、**外側のアクター隔離を引き継ぐ**。
View は `@MainActor` なので、この `Task` もメインアクターで始まる。

### `.task` は View の寿命に紐づく

```swift
.task { await model.load() }
```

`Task { }` と違い、**View が消えると自動でキャンセルされる**。
画面を閉じたあとに `model` を触ってクラッシュ、が起きない。

読み込みは基本これを使う。`onAppear` + `Task { }` にすると
キャンセルを自分で書くことになる。

## `defer` は Go と同じだが、対象が違う

```swift
func load() async {
    isWorking = true
    defer { isWorking = false }
    ...
}
```

Go は**関数**を抜けるとき、Swift は**スコープ**を抜けるとき。
`if` ブロックの中に書けばそのブロックの終わりで走る。

`async` 関数でも、途中で `throw` してもちゃんと走る。

## Go との対応

| | Go | Swift |
|---|---|---|
| 並行の単位 | goroutine | `Task` |
| 待つ | チャネル受信 / `WaitGroup` | `await` |
| 排他 | `sync.Mutex` | `actor` / `@MainActor` |
| 競合の検出 | `-race`（**実行時**） | 型検査（**コンパイル時**） |
| 「渡してよい」 | 規約 | `Sendable`（型で強制） |

**Go は「走らせてから壊れているか見る」、Swift は「壊れるコードを書かせない」。**
その代わり Swift は、既存コードを 6 系に上げるときに大量のエラーが出る。
