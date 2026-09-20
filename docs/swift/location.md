# 位置情報

「よく行く店で食べたものを上に出す」（要件 N-08）の仕組みと、CoreLocation で
つまずいたところ。

## 緯度経度はサーバに行かない

[docs/01-要件定義.md §4-C](../01-要件定義.md) で先に決めてある。

> **位置情報は端末内で処理し、サーバには送らない。** サーバが持つのは
> 「場所の名前」と、その場所に紐づく食事記録のみ。緯度経度は端末のローカルDBに置く。

なので `openapi.yaml` も DB も**変えていない**。並べ替えは端末内で完結する。

```
サーバ ──「よく使う順」で一覧を返す ──▶ iOS
                                        │
                        端末内の FoodPlaces で並べ替え
                                        │
                                        ▼
                              近くで使ったものが先頭
```

テストで固定してある。

```swift
@Test("**緯度経度を API に送らない**")
func neverSendsCoordinates() async {
    for req in t.requests {
        #expect(!(req.url?.absoluteString ?? "").contains("35.0"))
        #expect(!body.contains("lat"))
    }
}
```

## 座標は丸めて持つ

```swift
func coarse() -> Coordinate {
    Coordinate(lat: round(lat), lng: round(lng))   // 小数第3位
}
```

小数第3位は緯度で約 111m、東経 139 度あたりの経度で約 91m。
**店を区別するには足りるし、消し忘れたときの被害も小さい。**

生の緯度経度を端末に貯めると「いつどこにいたか」の履歴になる。
丸めてしまえば、そこまでの解像度は残らない。

## `sorted(by:)` は安定ではない

Swift の `sorted` は**同じ順位の要素の順を保証しない**。
「近くで使ったものを先頭に、残りはサーバの順のまま」をやるとき、これに当たる。

```swift
// だめ。同点のときに並びが暴れる
items.sorted { score($0) > score($1) }
```

前後に分けて、同点はもとの位置で決める。

```swift
var near: [(item: T, score: Int, at: Int)] = []
var rest: [T] = []

for (i, item) in items.enumerated() {
    let s = score(id(item), near: here)
    if s > 0 { near.append((item, s, i)) } else { rest.append(item) }
}

near.sort { $0.score == $1.score ? $0.at < $1.at : $0.score > $1.score }

return near.map(\.item) + rest
```

Python の `sorted` も JavaScript の `Array.prototype.sort`（ES2019 以降）も
**安定**なので、その感覚で書くと踏む。

## `requestLocation()` は delegate で返る

`async` の関数にしたいので `withCheckedContinuation` で包む。

```swift
func current() async -> Coordinate? {
    guard permission == .granted else { return nil }

    return await withCheckedContinuation { c in
        lock.withLock { waiting = c }
        manager.requestLocation()
    }
}

func locationManager(_ m: CLLocationManager, didUpdateLocations locations: [CLLocation]) {
    resumeWaiting(with: locations.last.map { ... })
}
```

**`didFailWithError` でも必ず resume する。** 片方を忘れると、次に
`current()` を呼んだところで永久に待つ。ここは失敗しても `nil` を返せば
「並び順が変わらない」だけなので握ってよい。

```swift
func locationManager(_ m: CLLocationManager, didFailWithError error: Error) {
    resumeWaiting(with: nil)
}
```

### `locationManagerDidChangeAuthorization` は `notDetermined` でも呼ばれる

delegate を設定した直後に1回来る。**ダイアログの答えではない。**
そのまま resume すると「聞く前に notDetermined が返る」ことになる。

```swift
func locationManagerDidChangeAuthorization(_ m: CLLocationManager) {
    guard permission != .notDetermined else { return }
    resumeAsking(with: permission)
}
```

### 追跡はしない

`startUpdatingLocation()` ではなく `requestLocation()`。
**1回ぶんの場所しか要らない。** 常時監視は電池を食うし、「いつも許可」を
要求する理由も無い。精度も `kCLLocationAccuracyHundredMeters` で足りる
（どうせ小数第3位に丸める）。

## 開いた瞬間に権限を聞かない

```swift
var canOfferNearby: Bool { location.permission != .denied && here == nil }

func enableNearby() async {
    if location.permission == .notDetermined { _ = await location.request() }
    here = await location.current()
}
```

一覧に「近い順に並べる」を1行置き、**押したときに初めて聞く。**

この画面は片手で速く触るのが狙い（#188）なので、開いた瞬間にシステムの
ダイアログで止めるのは筋が悪い。一度許可すれば以降は自動で並ぶ。

## UI テストでは本物を挿さない

**システムのダイアログは XCUITest から押せない**（`addUIInterruptionMonitor`
はあるが、タイミングが不安定）。`UITestSupport` で偽物に差し替える。

```swift
private var location: LocationSource {
    #if DEBUG
    if UITestSupport.isActive { return UITestSupport.makeLocation() }
    #endif

    return CoreLocationSource()
}
```

`HealthSource` と同じ形。**判断はすべて `FoodPlaces` / `MealModel` 側に
あり、そちらは `swift test` が見ている**ので、`CoreLocationSource` は
CoreLocation を呼ぶだけに留める。

SPM からは除外する（`Package.swift` の `exclude`）。CoreLocation は
macOS にもあるが、権限の聞き方が違う。

## 保存先

```swift
struct FoodPlaceStore: Sendable {
    let url: URL?    // Application Support/food-places.json
}
```

**`PendingQueue` ほど堅くしていない。** 送信待ちの記録は消えると困るが、
ここは消えても「並び順が戻る」だけ。記録そのものはサーバにある。

既定は `url: nil`（保存しない）にしてある。本物は `PhysiqueApp` が挿す。
**テストが実ファイルを触らないようにする**ためで、`HealthSource` と同じ考え方。
