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

## まだ無いもの

- **UI テスト**。タブの遷移やフォーカス移動は手で確認している
- **スナップショット**。上記の理由で入れていない
- **実機での HealthKit**。Personal Team では entitlement を付けられない
  （[build-config.md](build-config.md#personal-team-の制約)）
