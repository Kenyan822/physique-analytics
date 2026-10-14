# 画面の組み立てで踏んだこと

SwiftUI のレイアウトとタップ領域まわり。
**ここに書いてあるものは全部、実機かシミュレータでしか出なかった。**

## `.textFieldStyle(.roundedBorder)` の枠は文字の大きさで決まる

ジムで打つ入力欄が小さいので大きくしたかった（[#309](https://github.com/Kenyan822/physique-analytics/issues/309)）。
最初はこう書いた。

```swift
TextField("", text: binding)
    .textFieldStyle(.roundedBorder)
    .frame(width: 50, height: 44)   // ← 高さが効かない
```

**枠は大きくならない。** `.roundedBorder` は自分で背景を描き、その大きさは
フォントと内側の余白で決まる。`.frame(height:)` で広がるのは**タップ領域だけ**で、
見た目は変わらない。「押せるのに小さく見える」状態になる。

枠そのものを大きくするなら、スタイルを使わずに自分で描く。

```swift
TextField("", text: binding)
    .multilineTextAlignment(.center)
    .font(.title3.monospacedDigit())
    .frame(width: width, height: 44)
    .background(
        RoundedRectangle(cornerRadius: 8)
            .fill(Color(.secondarySystemBackground))
    )
    .overlay(
        RoundedRectangle(cornerRadius: 8)
            .stroke(Color(.separator), lineWidth: 1)
    )
```

色は `Color(.secondarySystemBackground)` / `Color(.separator)` のような
システム色にする。**自分で色を決めるとダークモードで破綻する。**

### 数字の欄は `monospacedDigit()`

```swift
.font(.title3.monospacedDigit())
```

等幅にしないと、`1` と `8` で文字幅が変わって**打つたびに枠の中で数字が揺れる。**
見た目の問題に見えるが、素早く打つときは意外と気が散る。

## タップ領域は 44pt

Apple のガイドラインの最小値。**狙わないと当たらない UI は、ジムでは使われない。**

この画面は過去に3回タップ事故を起こしている（[#202](https://github.com/Kenyan822/physique-analytics/issues/202)）。

- 引数ボタンが押せない
- キーボードを閉じないと他が押せない
- 量の数字が全部消せない

**どれも `swift test` も `xcodebuild build` も通っていた。** 型しか見ていないので
当然で、レイアウトは実行して初めて決まる。

## 幅を広げる前に、行の合計を数える

横並びの欄を大きくすると、黙って右端が切れる。エラーも警告も出ない。

```
16（番号）+ 68（重量）+ 18（kg）+ 14（×）+ 56（回数）
+ 18（回）+ 26（RIR）+ 46（RIR欄）+ 余白32 ≒ 294
```

List の行は左右に 20pt ずつ内側余白があるので、iPhone SE（375pt）でも収まる。
**一番狭い端末で引き算してから変える。**

## UI テストが見るのは「届くか」だけ

```swift
XCTAssertTrue(app.textFields["weightInput"].firstMatch.isHittable, "重量が打てる位置にあること")
```

枠の大きさを変えたら、**既存の `isHittable` が通ることを確認する。**
新しいテストを足すより、壊していないことを見る方が大事。

`isHittable` は「画面内にあって、他のものに隠れていないか」しか見ない。
**押しやすいかどうかは見ていない。** そこは実機でしか分からない。

## Form に行を1つ足すと、下の要素が「存在しない」ことになる

体組成に除脂肪体重の行を足したら（[#306](https://github.com/Kenyan822/physique-analytics/issues/306)）、
**関係ない UI テストが落ちた。**

```
XCTAssertTrue failed - 詳しく測るがあること
```

```swift
let more = app.buttons["showAllParts"]
XCTAssertTrue(more.exists, "詳しく測るがあること")   // ← ここで落ちる
app.swipeUp()
```

**`Form` は遅延描画する。** 画面の外にある行はビューが作られておらず、
アクセシビリティツリーにも出てこない。`isHittable` が false になるのではなく
**`exists` が false になる。**

行を1つ足して押し出しただけで、**触っていないテストが落ちる。**

```swift
app.swipeUp()
XCTAssertTrue(more.waitForExistence(timeout: UITimeout.normal), "詳しく測るがあること")
XCTAssertTrue(more.isHittable, "スクロールすれば押せること")
```

**スクロールしてから `exists` を見る。** 先に `exists` を見てからスクロールする
順番だと、画面の長さに依存したテストになる。

## Swift Charts の縦軸に 0 を含めない

```swift
.chartYScale(domain: .automatic(includesZero: false))
```

既定では 0 を含む。**体重 70〜72kg を 0 から描くと、変化が見えない直線になる。**
体組成のように「狭い範囲での動き」を見るグラフでは必ず外す。

### 横軸の整形を自動に任せない

```swift
.chartXAxis {
    AxisMarks(values: .automatic(desiredCount: 5)) { value in
        AxisValueLabel {
            if let d = value.as(Date.self) { Text(span.axisLabel(d)) }
        }
    }
}
```

既定の整形は**幅に応じて勝手に削る**。日ごとのグラフなのに月だけになったり、
2年ぶん並べたときに年が落ちて「どの6月か」が分からなくなる。

日ごとは `M/d`、月ごとは `yyyy/M` と決め打ちにした。

### タップした点の数値を出す（iOS 17 の `chartXSelection`）

```swift
@State private var tappedAt: Date?

.chartXSelection(value: $tappedAt)
```

返るのは**指のところの x 座標**で、点とは一致しない。一番近い点に寄せる。

```swift
static func nearest(_ points: [TrendPoint], to date: Date) -> TrendPoint? {
    points.min { abs($0.date.timeIntervalSince(date)) < abs($1.date.timeIntervalSince(date)) }
}
```

**何も選んでいないときは一番新しい点を出す。** 空にしておくと、
タップするまで数字が1つも読めない。

### UI テストで数値まで見るには、スタブにデータが要る

`/v1/daily` をスタブしていなかったので、グラフは常に「記録が無い」だった。
**グラフが出ない状態のテストは、グラフを検証していない。**

```swift
case ("GET", let p) where p.hasSuffix("/v1/daily"):
    return (["items": Self.dailyMetrics], 200)
```

値は公開してよい架空の数字にする（ADR-0002）。
**測っていない日を混ぜる**と、線が途切れないことも同時に見られる。

### `HStack` に付けた identifier は要素にならない

```swift
HStack { ... }.accessibilityIdentifier("trendReadout")   // ← 引けない
```

SwiftUI は**ただの `HStack` に要素を作らない**ので、`otherElements` にも
`staticTexts` にも出てこない。

```swift
Text(bodyText(p.value)).accessibilityIdentifier("trendValue")   // ← 引ける
```

**引きたいものそのものに付ける。** 入れ物ではなく中身。

### 月の平均が「何の平均か」を画面に書く

食事の推移を月ごとにすると、**1日あたりの平均**を出している。月の合計ではない。

```
6/1 に 1200、6/2 に 600 → 6月は 900（1800 ではない）
```

数字だけ見ると取り違える。桁が倍以上変わるので、画面に一行書いておく。

```swift
if model.span == .monthly {
    Text("月は1日あたりの平均").font(.caption).foregroundStyle(.secondary)
}
```

**記録が無い日は平均に入れない。** 入れると「食べなかった日」として
平均を押し下げるが、実際は入力し忘れただけのことが多い。

## 画面を足したら `Package.swift` の `exclude` にも足す

`swift test` は macOS 向けにビルドする（`.xcodeproj` との二重ビルド）。
iOS 専用の API を使うファイルは `exclude` しないと落ちる。

```
error: 'navigationBarTitleDisplayMode' is unavailable in macOS
```

**畳み方や計算はビューから出して core 側に置く。** そうすれば
`exclude` してもテストは効く（`TrendSeries.swift` は core、
`BodyChartView.swift` は exclude）。
