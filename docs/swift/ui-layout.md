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

### `chartXSelection` は単発のタップを拾わない

まずこう書いた。

```swift
.chartXSelection(value: $tappedAt)   // ← タップでは反応しない
```

**長押しドラッグでしか選べない。** 指で1回つついても何も起きない。
実機で触ってもらって「タップして数値出てこないよ」と言われた。

`chartOverlay` に `DragGesture(minimumDistance: 0)` を載せると、
タップもドラッグも同じ扱いになる。

```swift
.chartOverlay { proxy in
    GeometryReader { geo in
        Rectangle().fill(.clear).contentShape(Rectangle())
            .gesture(
                DragGesture(minimumDistance: 0)
                    .onChanged { v in
                        guard let frame = proxy.plotFrame else { return }
                        let x = v.location.x - geo[frame].origin.x
                        if let date: Date = proxy.value(atX: x) { tappedAt = date }
                    }
            )
    }
}
```

返るのは**指のところの x 座標**で、点とは一致しない。一番近い点に寄せる。

```swift
static func nearest(_ points: [TrendPoint], to date: Date) -> TrendPoint? {
    points.min { abs($0.date.timeIntervalSince(date)) < abs($1.date.timeIntervalSince(date)) }
}
```

**何も選んでいないときは一番新しい点を出す。** 空にしておくと、
タップするまで数字が1つも読めない。

### 「出ている」と「効いている」は違う

最初のテストはこうだった。

```swift
XCTAssertTrue(app.staticTexts["trendValue"].waitForExistence(timeout: .slow), "数値が出ること")
```

**通る。** タップが効いていなくても、未選択のときは一番新しい点を出して
いるので、数値そのものは最初から表示されている。
壊れた実装のままテストが緑だった。

実際にタップして、**状態が変わること**を見る。

```swift
XCTAssertTrue(app.staticTexts["trendLatestHint"].exists, "最新と出ていること")

let plot = app.otherElements["trendChart"].firstMatch
plot.coordinate(withNormalizedOffset: CGVector(dx: 0.25, dy: 0.5)).tap()

XCTAssertFalse(app.staticTexts["trendLatestHint"].exists, "選んだ点になること")
```

**書いたら、古い実装に戻して落ちることを確かめる。**
1回目は「要素が複数ある」という別の理由で落ちていて、狙いを見ていなかった。
`firstMatch` にして、意図した assertion で落ちるようにした。

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

## カメラは `UIImagePickerController` を包む

SwiftUI に**標準のカメラが無い**。`PhotosPicker` はライブラリ専用で撮影できない。

```swift
struct CameraPicker: UIViewControllerRepresentable {
    func makeUIViewController(context: Context) -> UIImagePickerController {
        let c = UIImagePickerController()
        c.sourceType = .camera
        c.delegate = context.coordinator

        return c
    }
}
```

**`Info.plist` に `NSCameraUsageDescription` が要る。** 無いと権限ダイアログが
出ないのではなく、**プロセスが落ちる。** 書き忘れると実機で初めて分かる。

シミュレータにカメラは無いので、`UIImagePickerController.isSourceTypeAvailable(.camera)`
で出し分ける。出しっぱなしだと、シミュレータで押せるのに何も起きない。

## 画像の縮小は ImageIO で書く（UIKit で書かない）

```swift
CGImageSourceCreateThumbnailAtIndex(src, 0, [
    kCGImageSourceCreateThumbnailFromImageAlways: true,
    kCGImageSourceThumbnailMaxPixelSize: maxPixels,
    kCGImageSourceCreateThumbnailWithTransform: true,   // 撮った向きのまま
] as CFDictionary)
```

`UIImage` で書くと iOS 専用になり、`Package.swift` で `exclude` することになる。
**exclude したファイルは `swift test` から見えない。** 誰も検証しないコードになる。

ImageIO は macOS でも動くので、**縮小の判断（長辺・縦横比・拡大しない）を
テストできる。** 画面に出す部分だけ iOS 専用にして、判断は core に置く。

`kCGImageSourceCreateThumbnailWithTransform` を忘れると、
**縦で撮った写真が横倒しで送られる。** 量が読み取りにくくなる。

## 待たせる処理は「止める口」を残す

解析中に他を触らせないのは正しいが、**閉じられない画面は通信が詰まると
閉じ込めになる。** 「やめる」を置いて `Task` を cancel できるようにする。

スワイプで閉じさせないのは `.interactiveDismissDisabled()` でやる。
閉じる口はボタンだけにする。

やめたときにエラーを出さないこと。自分で止めたのに「キャンセルされました」が
出るのは雑音になる。`URLSession` は中断を `CancellationError` ではなく
`URLError.cancelled` で返すので、**両方を見る**。

```swift
private func isCancellation(_ error: Error) -> Bool {
    if error is CancellationError { return true }
    if case .transport(let e) = error as? APIError,
       (e as? URLError)?.code == .cancelled { return true }

    return (error as? URLError)?.code == .cancelled
}
```

## モーダルを閉じながら別のモーダルを出すと、後の方が消える

写真に補足を書くシートを閉じて、解析中の `fullScreenCover` を出す作りにした。

```swift
// 補足を書くシート
.sheet(isPresented: Binding(get: { pendingImage != nil }, set: ...)) { photoNoteSheet }
// 解析中
.fullScreenCover(isPresented: Binding(get: { model.estimating }, set: { _ in })) {
    estimatingCover
}

private func runEstimate() {
    pendingImage = nil       // ここでシートが閉じ始める
    estimateTask = Task {    // ほぼ同時に estimating が true になる
        await model.estimate(...)
    }
}
```

**実機で解析中の画面が1度も出なかった。** ビルドもテストも緑だった。

同じ view に付いた presentation は同時に1つしか動かない。閉じるのと
出すのが同じ瞬間に重なると、**SwiftUI は後から出す方を黙って捨てる。**
エラーも警告も出ない。

直し方は**presentation を増やさないこと。** シートは1枚のままにして、
中身だけ差し替える。

```swift
private enum PhotoFlow {
    case note(Data)
    case estimating
}
@State private var photoFlow: PhotoFlow?

.sheet(isPresented: Binding(
    get: { photoFlow != nil },
    set: { if !$0 { cancelPhotoFlow() } }
)) { photoSheet }
```

これなら `isPresented` は一度も false にならないので、閉じて開き直しが起きない。

## 「出ている」と「押せる」の間に「閉じかけ」がある

上の直しを入れたあと、UI テストはこう落ちた。

```
XCTAssertTrue failed - 押せる位置にあること
Button, {{166.4, 1128.1}, {69.1, 33.0}}, identifier: 'cancelEstimate'
```

画面は 874pt しかないので、y=1128 は画面の外。**レイアウトを疑って
2回直したが、どちらも直らなかった**（裸の VStack をやめて NavigationStack に
入れる → さらに Form の行にする）。y は実行のたびに 1128 → 1161 → 1072 と動いた。

動いているなら静的なレイアウトの問題ではない。**シートが閉じるアニメーションの
途中を掴んでいた。**

原因はテスト側のスタブ。

```swift
if path.hasSuffix("/v1/meals/estimate") {
    try? await Task.sleep(for: .seconds(3))   // 短すぎた
}
```

**XCUITest は要素を1つ探すのに1秒前後かける。** `waitForExistence` が
通ってから `isHittable` を見るまでに数秒経つので、3秒で終わる処理は
その間に完了して、シートが閉じ始めている。

待たせる処理のスタブは**テストの手数より十分に長く**する。テストは
「やめる」を押して打ち切るので、長くしても待たされない。

| 症状 | 疑う順 |
|---|---|
| 要素が無い | 本当に出していない |
| 要素はあるが画面外で、**座標が実行ごとに動く** | アニメーションの途中を見ている |
| 要素はあるが画面外で、**座標が毎回同じ** | レイアウト |

## 逃げ道を主操作と入れ替えない

キーボードを閉じる道は3つ用意してあった。

| | いつ効くか |
|---|---|
| スクロール | `scrollDismissesKeyboard(.interactively)` |
| タップ | **読むだけの行**を押したとき |
| 「完了」 | 上2つが効かない場所の逃げ道 |

実機で「他のところをタップしても閉じない」と言われた。

3つ目が消えていた。

```swift
if let action {
    Button(action.title) { ... }      // 「記録」
} else {
    Button("完了") { focus.wrappedValue = nil }
}
```

`action` は「下書きが空でなければ記録」なので、**1文字でも打った瞬間に
「完了」が「記録」に置き換わる。** 閉じたくなるのは打っている最中なので、
要るときにだけ無い状態になっていた。

主操作と逃げ道は**排他にしない。** 両方出す。

```swift
Spacer()
Button("完了") { focus.wrappedValue = nil }
if let action {
    Button(action.title) { ... }.bold()
}
```

2つ目（タップ）も狭すぎた。`dismissesKeyboardWhenTapped()` を
セクションの**見出しの文字にだけ**付けていたので、当てに行かないと当たらない。
行の中身にも付けた。

**「経路が3つある」と書いてあっても、同時に3つ効いているとは限らない。**

## 「打っている最中に押せるか」で置き場所が決まる

食事の入力にリセットを足した（[#320](https://github.com/Kenyan822/physique-analytics/issues/320)）。
最初は記録ボタンの横に並べた。**行を増やさないので安全**だと思った。

UI テストで落ちた。

```
XCTAssertTrue failed - 記録が押せる位置にあること
```

調べたら、**記録ボタンはもともとキーボードの下にある。** この画面は
キーボード上のバーから記録する作りになっていた（#202 の結果）。

```swift
.keyboardFocusBar(focus: $focus, order: Field.allCases,
    action: model.draft.isEmpty ? nil : ("記録", { Task { await model.record() } }))
```

**やり直したくなるのは打っている最中。** 記録ボタンの横に置くと、
一度キーボードを閉じないと押せない。ツールバーのメニューに移した。

| 置き場所 | キーボードが出ていても押せるか |
|---|---|
| Form の中 | **押せない**（下に隠れる） |
| キーボード上のバー | 押せる（ただし1つしか置けない） |
| **ツールバー** | **押せる** |

### 取り消せるなら確認は要らない

消す前に「本当に消しますか」を出すと、**急いでいるときに2タップになる。**
押してから戻せる方が速い。

```swift
var canUndoReset: Bool { undoableDraft != nil && draft.isEmpty }
```

**打ち直したあとは出さない。** そこで戻すと、打った内容が消える。
「空のあいだだけ戻せる」にしておくと、取り違えが起きない。
