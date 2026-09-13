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

## HealthKit（要件 B-01 / B-09）

コードは入っている（`HealthSync.swift` / `HealthKitSource.swift`）が、
**実機でしか動かないので未検証。**

| | 状態 |
|---|---|
| 取り込みの判断（日ごとの集約・単位変換・差分取得） | `swift test` で検証済み |
| HealthKit の API 呼び出し | 型チェックのみ。**シミュレータではデータが空で検証にならない** |
| Info.plist の利用目的 | 記載済み（読み取りのみ） |
| **entitlement** | **未設定。実機で動かす前に要る** |

### 実機で動かすときの手順

1. Xcode で `Physique` ターゲット → Signing & Capabilities
2. `+ Capability` → **HealthKit** を追加
   （`Physique.entitlements` が生成され、プロビジョニングプロファイルが更新される）
3. 実機で起動し、体組成タブの「Apple Health から取り込む」を押す

**ここを手順として残しているのは、entitlement を先に書き込むと
CI のシミュレータビルドが壊れるかどうかを手元で確かめられないため。**
Xcode に iOS プラットフォームが入っていない環境では `xcodebuild` が動かない。

### 設計

- **読み取り専用。** 書き込みの権限は要求しない。このアプリが Apple Health を汚す理由が無い
- **自動では取り込まない。** 起動のたびに権限ダイアログが出るのは邪魔だし、
  いつ何が入ったか分からないまま数字が変わる方が怖い
- **差分だけ取る。** 最後に取り込んだ日の翌日から。初回は30日前から
  （HRV の基準窓が30日）
- **手入力を上書きしない。** 送るのは HealthKit から来た項目だけで、
  疲労度のような手入力の項目は触らない（API 側で nil は「変更しない」）
- `inBed` を睡眠時間に含めない。ベッドにいた時間は睡眠時間ではない
