# Swift Testing —— 何をどう検証しているか

93 件。すべて `swift test`（SPM）で走り、**Xcode もシミュレータも要らない**。
実行は 0.1 秒未満。

## XCTest ではなく Swift Testing

```swift
import Testing

@Suite("食事の区分")
struct MealSlotTests {
    @Test("時刻から区分を推測する")
    func suggests() {
        #expect(slot(hour: 7) == .breakfast)
    }
}
```

| | XCTest | Swift Testing |
|---|---|---|
| 型 | `class: XCTestCase` | **`struct` でよい** |
| 検証 | `XCTAssertEqual(a, b)` | `#expect(a == b)` |
| 名前 | メソッド名（英数字） | `@Test("日本語で書ける")` |
| 並列 | 既定は直列 | **既定で並列** |
| 失敗時 | 期待値と実際値 | **式を分解して表示** |

### `#expect` はマクロなので式が見える

```
Expectation failed: (slot(hour: 4) → .lunch) == .snack
```

`XCTAssertEqual` だと `("lunch") is not equal to ("snack")` としか出ない。
**`slot(hour: 4)` が何を返したか**まで出るので、デバッガを開かずに済む。

### `#require` は「これが無いなら先は無意味」

```swift
let url = try #require(t.requests.first?.url)
#expect(url.path == "/v1/exercises")
```

`#expect` は失敗しても続行するが、`#require` は throw してそのテストを止める。
`nil` のまま次を評価しても `nil` 起因の失敗が連鎖するだけなので、ここで切る。

XCTest の `XCTUnwrap` に相当する。

## 偽物を注入する

### `HTTPTransport`

```swift
/// 記録した URLRequest を返すだけの偽 transport。
/// 実 API に繋ぐとテストがネットワークと DB の状態に依存する。
final class FakeTransport: HTTPTransport, @unchecked Sendable {
    private(set) var requests: [URLRequest] = []
    var responses: [(Data, Int)] = []
    var error: Error?

    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse) {
        requests.append(request)
        if let error { throw error }

        let (data, status) = responses.count > 1 ? responses.removeFirst() : responses[0]
        ...
    }
}
```

**`requests` を溜めているのが肝。** 応答を差し替えるだけでなく、
**送った内容を検証できる**。

```swift
@Test("部位で絞ると日本語がエンコードされる")
func filterByMuscleGroup() async throws {
    _ = try await api.listExercises(muscleGroup: .chest)

    let url = try #require(t.requests.first?.url)
    #expect(url.query?.contains("%E8%83%B8") == true)
}
```

`responses.count > 1 ? removeFirst() : responses[0]` は、
**1つだけ入れたら何回呼んでも同じものを返す**という手抜き。
複数入れたら順に消費する。呼び出し回数に応じて変える必要があるテスト
（リトライなど）だけ複数入れる。

### `SessionStore` / `PendingStore` / `HealthSource`

同じ形で、すべて protocol + メモリ実装。

```swift
final class MemoryPendingStore: PendingStore, @unchecked Sendable {
    private var data: Data?
    func load() -> Data? { data }
    func save(_ data: Data) { self.data = data }
    func clear() { data = nil }
}
```

`HealthSource` は特に効く。

```swift
/// **インターフェースにしてあるのは、実機が無くても取り込みの判断を
/// テストできるようにするため。** HealthKit はシミュレータでは
/// データが空なので、本物では検証にならない。
protocol HealthSource: Sendable {
```

「歩数は合計、体重は最後の値」のような**畳み込みの判断**は
`HealthSync.toDaily` に切り出してあり、純粋関数としてテストできる。

## `@MainActor` なモデルのテスト

```swift
@Suite("食事の記録")
@MainActor
struct MealModelTests {
    @Test("読み込むと合計と残量が出る")
    func load() async {
        let model = MealModel(api: api, date: "2026-09-16")
        await model.load()

        #expect(model.totals.kcal == 800)
    }
}
```

**`@Suite` に `@MainActor` を付ける。** 付けないと
`MealModel` のプロパティに触れずコンパイルが通らない。

## 時刻を引数にする

```swift
init(api: APIClient, date: String = JST.dateString(), now: Date = Date()) {
    self.date = date
    self.slot = MealSlot.suggested(at: now)
}
```

`Date()` を内部で呼ぶと、**テストが実行時刻に依存する**。
既定引数にしておけば本番のコードは変わらず、テストだけ固定値を渡せる。

`AuthClient.signIn(email:password:now:)` も同じ形。

```swift
func signIn(email: String, password: String, now: Date = Date()) async throws -> Session
```

## 踏んだ罠

### タイムゾーン依存（#171）

```swift
// ❌ 端末のタイムゾーンで Date を作る
let date = Calendar(identifier: .gregorian).date(from: c)!
```

JST の手元では通り、**UTC で走る CI では9時間ずれて11件落ちた**。

```swift
/// **JST で組み立てる。** 素の `Calendar` は端末のタイムゾーンで Date を作るので、
/// UTC で走る CI だと9時間ずれて全ケースが落ちる。
private func slot(hour: Int) -> MealSlot {
    var c = DateComponents()
    c.year = 2026; c.month = 9; c.day = 16; c.hour = hour
    let date = JST.calendar.date(from: c)!

    return MealSlot.suggested(at: date)
}
```

確認は環境変数で。

```console
$ TZ=UTC swift test
$ TZ=Asia/Tokyo swift test
$ TZ=America/New_York swift test
```

**手元が JST だけなのは、テストの半分を実行していないのと同じ。**

同じ種類のバグが Web 側にもあった（#175）。そちらは SSR と
ブラウザでタイムゾーンが違うことによる hydration mismatch。

### 必須フィールドを省いた偽 JSON

`Problem` の `type` は必須にしてあるので、テストの偽 JSON で省くと
デコードに失敗する。**テスト側の間違い**で、実装は正しかった。

偽データは「API が実際に返す形」をそのまま書く。省略すると、
本番では起きないデコード失敗を追うことになる。

### `Int(number(kcal) ?? .nan)`

`Double.nan` を `Int` に変換すると**実行時クラッシュ**する
（Swift は変換不能を未定義動作にせず落とす）。

```swift
private func int(_ s: String) -> Int? {
    number(s).map { Int($0) }     // nil なら nil のまま
}
```

`?? .nan` でごまかさず、`Optional` のまま運ぶ。

## CI の構成

```yaml
# ロジックは Xcode を通さずに回す。速いし、UI に依存しない部分を切り分けられる
- name: Test（SPM）
  run: swift test

# 画面まで含めてコンパイルが通ることを確かめる
- name: Build（シミュレータ）
  run: xcodebuild -project Physique.xcodeproj -scheme Physique \
         -destination 'platform=iOS Simulator,name=iPhone 17' build
```

**UI は「コンパイルが通る」までしか見ていない。** SwiftUI の
スナップショットテストは、レイアウトの微差で落ちて維持コストが高い。
[CLAUDE.md](../../CLAUDE.md) でも UI のテストは必須にしていない。

代わりに**判断を View から追い出して**、モデル側で全部テストしている
（[app-structure.md](app-structure.md#この分け方の代償)）。

## 「押せるか」は XCUITest で見る（#202）

`swift test` は**押せるかを見ていない**。`MealModel.record()` が正しくても、
ボタンがキーボードの下にあれば使えない。

### 実際に何を捕まえたか

キーボードバーに「記録」を置く前、このテストが落ちた。

```
XCTAssertTrue failed - 記録ボタンがキーボードに隠れていないこと
```

**報告される前に機械が見つけた。** `Form` の中のボタンは、キーボードが
上がると隠れる。目標シートの保存ボタンで踏んだのと同じ原因。

### `isHittable` が肝

```swift
let record = app.buttons["keyboardPrimaryAction"]
XCTAssertTrue(record.waitForExistence(timeout: 5))
XCTAssertTrue(record.isHittable, "記録がキーボードに隠れていないこと")
```

| | 何を見るか |
|---|---|
| `exists` | 要素が**ある**か |
| **`isHittable`** | **その座標を押したら本当にその要素に届くか** |

`exists` だけだと、**画面外にあっても通る**。

逆に **`List` / `Form` の中では `exists` も当てにならない** ——
行を遅延生成するので、スクロールして初めて生える
（[food-master.md](food-master.md#form-は見えていない行を作らない)）。

### 名前で引かない。識別子を振る

「記録」は3つあった。

| | |
|---|---|
| タブバーの「記録」 | トレーニング記録タブ |
| `Form` の中の「記録」 | キーボードに隠れる |
| キーボードバーの「記録」 | |

`app.buttons["記録"]` はこのどれかを掴む。どれかは**実行するまで分からない**。

```swift
.accessibilityIdentifier("recordButton")
.accessibilityIdentifier("keyboardPrimaryAction")
```

`Num` のような自作の部品も同じ。ラベル（`Text`）と `TextField` が別要素なので、
ラベルでは引けない。**並び順（`element(boundBy:)`）も脆い** —— 欄が1つ増えるとずれる。

### 認証と通信を迂回する

**`#if DEBUG` ＋ 起動引数。** Release ビルドには入らない。

```swift
#if DEBUG
if UITestSupport.isActive {
    _auth = State(initialValue: UITestSupport.makeAuth())
    return
}
#endif
```

本番の経路は変えない。既存の `HTTPTransport` / `SessionStore` の protocol に
偽物を挿すだけで、**画面側のコードは触らない**。

偽の transport は**記録した内容を覚える**。「記録したら一覧に出る」を見たいので、
毎回同じものを返すだけでは足りない。

### `NSLock` は async の中でロックできない

```swift
// ❌ Swift 6 でコンパイルエラー
func send(_ r: URLRequest) async throws -> (Data, HTTPURLResponse) {
    lock.lock(); defer { lock.unlock() }
```

```
error: instance method 'lock' is unavailable from asynchronous contexts
```

`withLock` なら同期のスコープに閉じるので通る。

```swift
let (json, status) = lock.withLock {
    respond(path: path, method: method, body: body)
}
```

### テストが機能するかを確かめる

**バグを意図的に戻して、落ちることを見る。**

```console
$ # キーボードバーから「記録」を外す
$ xcodebuild test -only-testing:PhysiqueUITests/MealInputTests/test_PFCを入れて記録できる
** TEST FAILED **
```

落ちなければ、そのテストは何も守っていない。`swift test` の Red を確認するのと
同じ理由（[CLAUDE.md](../../CLAUDE.md#失敗を確認する理由)）。

### 対象を広げない

| | 1本あたり |
|---|---|
| `swift test` 124件 | **合計 0.02 秒** |
| XCUITest 6本 | **合計 95 秒**（1本 6〜32秒） |

桁が違う。**ロジックは `swift test` が見ている**ので、ここで見るのは
「届くか」だけにする。

### CI

```yaml
- name: UI テスト（シミュレータ）
  run: |
    set -o pipefail
    xcodebuild test ... | xcbeautify
```

**`set -o pipefail` が要る。** 無いと `xcbeautify` の終了コードだけが見られ、
テストが落ちても CI が緑になる。

落ちたときは `xcresult` を artifact に残す。CI のログだけだと
`isHittable failed` としか分からないが、`xcresult` にはスクリーンショットが入る。

## まだ無いもの

- **スナップショット**。レイアウトの微差で落ちて維持コストが高い
- **Web の UI テスト**。Playwright で本番を見る運用にしている
- **実機での HealthKit**。Personal Team では entitlement を付けられない
  （[build-config.md](build-config.md#personal-team-の制約)）
