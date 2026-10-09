# HealthKit

体重・体脂肪率・歩数・睡眠・HRV を Apple Health から取り込む（要件 B-01 / B-09）。
実装は `HealthKitSource`（HealthKit を叩く）と `HealthSync`（日次に畳む）に分かれている。

ここに書いてあるのは**全部、実機で踏んだあとに分かったこと**。
`swift test` も `xcodebuild build` も、このどれも検出しなかった。

## `HKUnit.percent()` は百分率ではなく割合を返す

```swift
let raw = q.quantity.doubleValue(for: .percent())   // 20% なら 0.2
```

体脂肪率をそのまま保存すると **0.2%** になる。実際に本番へ 0.2 が入った（#291）。

```swift
/// 取り出す単位。**ここを間違えると静かに桁がずれる。**
private func scale(_ kind: HealthKind, _ v: Double) -> Double {
    kind == .bodyFatPercentage ? v * 100 : v
}
```

**この手のズレは型では止まらない。** `Double` から `Double` なので、
コンパイラにも `swift test`（偽物の `HealthSource` を使う）にも、
間違いだと分かる材料が無い。単位は `HKUnit` を選ぶ1か所に集めて、
そこにコメントを置くしかない。

同じ理由で、秒で返るものも1か所で直している。

| 種類 | HealthKit が返す単位 | 保存する単位 |
|---|---|---|
| 体脂肪率 | 割合（0.2） | % （20.0） |
| HRV (SDNN) | 秒 | ms |
| 睡眠 | 秒 | 時間 |
| 深睡眠 | 秒 | 分 |

なお、**壊れた値は後から直せないことがある。** `bodyfat_pct` は `numeric(4,1)` なので、
割合のまま入った値は小数第1位に丸められてしまう。100倍して戻すことはできない
（元の桁が残っていない）。null にして取り込み直した。

## entitlement が無いと認可要求が落ちる

```
Missing com.apple.developer.healthkit entitlement
```

`HKHealthStore.requestAuthorization` はこれで throw する。
**Info.plist に使用目的を書くだけでは足りない。**

Xcode で "Signing & Capabilities" から HealthKit を足せば自動で入るが、
このプロジェクトは `project.pbxproj` を手で持っているので、ファイルを作って
両方の build configuration に配線する必要があった。

```
ios/Physique/Physique.entitlements
```

```
CODE_SIGN_ENTITLEMENTS = Physique/Physique.entitlements;   // Debug / Release の両方に書く
```

片方だけに書くと、そのコンフィギュレーションのときだけ落ちる。

入ったかどうかは**署名済みの .app を見る**のが確実。ソースを見ても、
pbxproj の配線が効いているかは分からない。

```bash
codesign -d --entitlements - path/to/Physique.app
```

### 読み取り専用なら `NSHealthUpdateUsageDescription` を書かない

`toShare: []` で要求しているので、要るのは `NSHealthShareUsageDescription` だけ。
書き込みの説明文を足すと、要求していない権限の確認が出る。

## 読み取り権限が下りたかは問い合わせられない

`authorizationStatus(for:)` が答えるのは**書き込み**についてだけ。
読み取りは、許可されていても拒否されていても同じに見える。

これは仕様で、**アプリが「この人はこのデータを隠した」と知れてしまうのを防いでいる**。
拒否されたときは「データが無い」のと区別がつかない形で空が返る。

結果、「もう許可をもらったか」はアプリ側で覚えるしかない。

```swift
private(set) var healthGranted: Bool {
    get { defaults.bool(forKey: "healthGranted") }
    set { defaults.set(newValue, forKey: "healthGranted") }
}
```

`UserDefaults` は**注入できるようにしておく**。`.standard` を直に触ると、
テストどうしが同じ値を共有して、実行順で結果が変わる（実際に踏んだ）。

```swift
init(api: APIClient, health: HealthSource? = nil, defaults: UserDefaults = .standard, ...)
```

## シミュレータでは検証にならない

HealthKit はシミュレータだとデータが空なので、本物を挿しても何も取れない。
加えて **UI テストでは本物を挿してはいけない** —— システムの確認ダイアログが出て、
テストから押せなくなる。CI の素のシミュレータでこれを踏んだ。

そのため `HealthSource` を protocol にして、取り込みの判断（`HealthSync`）だけを
偽物でテストしている。

```swift
protocol HealthSource: Sendable {
    func requestAuthorization() async throws
    func samples(for kind: HealthKind, from: Date, to: Date) async throws -> [HealthSample]
}
```

位置情報と同じ形（[location.md](location.md)）。**OS が確認ダイアログを出すものは、
一度 protocol で包む**と憶えておくのが早い。

## 同じ日に複数あるとき、合計か最後か

```swift
var aggregation: Aggregation {
    switch self {
    case .stepCount, .sleepDuration, .deepSleepDuration: .sum
    case .bodyMass, .bodyFatPercentage, .hrv, .restingHeartRate: .last
    }
}
```

歩数と睡眠は区間ごとに入るので、合計しないと過少になる。
体重は1日に何度も乗ることがあるので、足すと無意味な値になる。

**値が1つも無い日は行を作らない。** 空の行を作ると「測ったが0だった」と
区別できなくなる。

## 睡眠は「ベッドにいた時間」を含めない

`HKCategoryValueSleepAnalysis` には `inBed` がある。これは睡眠時間ではない。

```swift
return value == .asleepCore || value == .asleepDeep
    || value == .asleepREM || value == .asleepUnspecified
```

## 取り込みのメッセージが消えていた話

`syncHealth` が `message` を立てた直後に `load()` を呼んでいて、
`load()` の先頭が `message = nil` だった。**エラーが出ていたのに画面に残らなかった。**

```swift
await load(date: today)
message = "取り込みました"      // load のあとに立てる
```

これと「メッセージが `Form` の一番下にあり、ボタンは一番上だった」が重なって、
entitlement が無いという明確なエラーが**何週間も見えていなかった**。

テストでも一度すり抜けた。`#expect(m.message != nil)` は、`load()` が立てた
**別のエラーメッセージ**で通ってしまう。内容まで見る。

```swift
#expect(m.message?.contains("取り込") == true)
```
