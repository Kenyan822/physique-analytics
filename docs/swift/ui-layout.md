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
