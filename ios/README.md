# iOS

ジムでの入力に特化したクライアント（[ADR-0009](../docs/adr/0009-web-first-with-input.md)）。
**Web と役割を分ける。** Web は入力も分析もできる汎用クライアントで、
iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」。

## 構成

| | |
|---|---|
| `Physique/Sources/` | アプリ本体。`.xcodeproj` は `PBXFileSystemSynchronizedRootGroup` なので、**ファイルを置けば自動で入る** |
| `PhysiqueTests/` | `swift test` で回すテスト |
| `Package.swift` | Xcode を開かずにロジックを回すための SPM 定義 |

## テストとビルド

```bash
swift test                 # ロジック。速いので普段はこれ
```

UI（`*View.swift` と `PhysiqueApp.swift`）は SPM の対象から外している。
SwiftUI の iOS 専用 API（`.keyboardType` など）が macOS ビルドで落ちるため。

### UI のコンパイルを手元で確かめる

Xcode に iOS プラットフォームが入っていないと `xcodebuild` は動かないが、
**シミュレータ SDK だけで型チェックはできる。**

```bash
SDK=$(xcrun --sdk iphonesimulator --show-sdk-path)
xcrun --sdk iphonesimulator swiftc -typecheck \
  -target arm64-apple-ios17.0-simulator -sdk "$SDK" Physique/Sources/*.swift
```

CI（`ios (Swift)`）は `swift test` に加えてシミュレータ向けの
`xcodebuild build` まで通す。

## 画面

| | 要件 |
|---|---|
| 記録（`LogView`） | T-01〜T-04。片手・手袋・汗で使えることが最優先 |
| 体組成（`BodyView`） | B-02 / B-03 / B-06。朝に触るものなのでタブを分けている |

## まだ無いもの

**HealthKit 連携（要件 B-01 / B-09）は実機が要る**ので未着手。
体重・体脂肪率・歩数・睡眠・HRV・安静時心拍を自動取得する経路になる。
