# ios

ジムでの入力に特化したクライアント（[ADR-0009](../docs/adr/0009-web-first-with-input.md)）。

**Web と役割を分ける。** Web は入力も分析もできる汎用クライアントで、
iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」。

## 構成

```
ios/
├── Physique.xcodeproj/     アプリ本体
├── Package.swift           ロジックを swift test で回すためのパッケージ
├── Physique/Sources/
│   ├── PhysiqueApp.swift   エントリポイント
│   ├── LogView.swift       入力画面（SwiftUI）
│   ├── LogModel.swift      画面の状態。UI から切り離してテストする
│   ├── APIClient.swift     API クライアント
│   ├── PendingQueue.swift  オフラインの記録キュー（要件 T-07）
│   ├── Models.swift        openapi.yaml に対応する型
│   └── JST.swift           JST 固定の日付（ADR-0013）
└── PhysiqueTests/          swift test で回る
```

`.xcodeproj` は **file-system-synchronized group** を使っている（Xcode 16 以降）。
ファイルを足しても `project.pbxproj` を触らなくてよく、生成ツール（XcodeGen 等）も要らない。

## 動かす

API が先に要る。

```bash
# リポジトリルートで
docker compose up -d
docker compose run --rm migrate up
cd api && DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' \
  AUTH_DISABLED=true go run ./cmd/server
```

```bash
cd ios
swift test                                    # ロジックのテスト（Xcode 不要）
open Physique.xcodeproj                       # GUI
```

シミュレータへはコマンドからも入れられる。

```bash
xcodebuild -project Physique.xcodeproj -scheme Physique \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' \
  -derivedDataPath /tmp/dd build

xcrun simctl boot 'iPhone 17 Pro'
xcrun simctl install booted /tmp/dd/Build/Products/Debug-iphonesimulator/Physique.app
xcrun simctl launch booted com.example.physique
```

シミュレータからは `localhost:8080` がそのまま届く。
実機で試すときは `Info.plist` に `API_BASE_URL` を足す。

## 実装している要件

| ID | 内容 |
|---|---|
| T-01 | 種目を選び、重量・レップ・RIR を記録する |
| **T-02** | **前回の値をデフォルト表示する**。前回の推定1RM も出す |
| T-03 | 重量は 2.5kg 刻みのステッパー（プレートの最小単位が 1.25kg × 2） |
| T-04 | 記録すると次のセット番号になる |
| T-05 | インターバル（多関節 180 秒 / 単関節 90 秒） |
| T-07 | オフラインで記録し、復帰時に送る |

## まだやっていないこと

- HealthKit 連携（体重・歩数・睡眠・HRV・安静時心拍）
- SwiftData でのローカル永続化（今はキューのみファイル保存）
- テンプレート呼び出し（T-06）
- 認証（Supabase のログイン）
- swift-openapi-generator への移行。今は手書きの薄い層

## なぜ SPM パッケージも置いているか

`swift test` を Xcode 抜きで回すため。CI でも手元でも速く、UI に依存しない
ロジック（日付・API・キュー）をここで固めてから画面に繋ぐ。
