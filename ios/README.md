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

## 実機で動かす

### 1. 接続先と鍵を入れる

```bash
cp Physique/Config.xcconfig.example Physique/Config.xcconfig
```

| 変数 | 中身 |
|---|---|
| `API_BASE_URL` | Cloud Run の URL。**未設定だと `localhost:8080` を見にいき、実機からは届かない** |
| `SUPABASE_URL` | Supabase のプロジェクト URL |
| `SUPABASE_ANON_KEY` | 公開前提の鍵。単体では何も読めない（DB は RLS で閉じている） |
| `DEVELOPMENT_TEAM` | 署名チーム（10桁）。次項 |

**Xcode での割り当て操作は要らない。** `Physique/Base.xcconfig` がプロジェクトに
接続済みで、その末尾から `#include? "Config.xcconfig"` している。
`?` 付きなのでファイルが無くてもビルドは通る（既定値で動く）。

`Config.xcconfig` は `.gitignore` に入れてある。実値を public リポジトリに入れない。

### 2. Apple ID を入れて Team ID を調べる

無料の Apple ID で足りる（**7日で切れる**。§「7日で切れる」）。

1. `Xcode > Settings > Accounts` で Apple ID を追加する
2. `Physique.xcodeproj` を開き、`TARGETS > Physique > Signing & Capabilities`
3. `Team` に `(自分の名前) (Personal Team)` を選ぶ
4. 同じ画面に出る **Team ID（10桁）** を `Config.xcconfig` の `DEVELOPMENT_TEAM` に書く

**4 をやったら 3 の GUI 設定は戻してよい。** GUI で設定すると `project.pbxproj`
（commit 対象）に個人の Team ID が書き込まれるため、xcconfig 側で渡す。

bundle ID は `io.github.kenyan822.physique`。**`com.example.*` にしない** ——
Personal Team は App ID を自動登録するので、他人が押さえている名前だと
`The app identifier cannot be registered` で止まる。

### 3. iPhone 側の準備

1. USB で Mac に繋ぎ、「このコンピュータを信頼しますか？」に **信頼**
2. iPhone の `設定 > プライバシーとセキュリティ > デベロッパモード` を **オン**（再起動する）
3. Xcode 左上のデバイス選択で自分の iPhone を選ぶ
4. **⌘R**

初回起動時に「信頼されていないデベロッパ」と出たら、
iPhone の `設定 > 一般 > VPNとデバイス管理` で自分の Apple ID を **信頼**する。

### 7日で切れる

無料アカウントの provisioning profile は **7日で失効**し、アプリが起動しなくなる。
切れたら Xcode から ⌘R で入れ直す。**記録は消えない**（データは Cloud Run の先にある）。

有料の Apple Developer Program（年 $99）に入れば1年になるが、
[docs/05-インフラ設計.md](../docs/05-インフラ設計.md) の方針では当面申請しない。

### HealthKit はまだ動かない

`Apple Health から取り込む` は失敗する。HealthKit の entitlement は
**Personal Team では付けられない**ため。押しても落ちず、理由が画面に出るだけで、
体組成の手入力・食事・トレーニング記録は動く。有料アカウントに移すときに対応する。

## ログイン

Supabase Auth（メール＋パスワード）。**SDK は入れていない** —— 使うのは
ログイン・取り直し・ログアウトの3つだけで、アプリは既に `HTTPTransport` を
持っている。依存を1つ増やすより既存の層に載せる方が、差し替えもテストも効く。

| | どこ |
|---|---|
| Supabase を叩く | `AuthClient.swift` |
| ログイン状態 | `AuthModel.swift`（`@Observable`） |
| 保存 | `SessionStore.swift`（Keychain） |
| 画面 | `LoginView.swift` |

**アクセストークンは `AuthModel.accessToken()` からしか取らない。** 各画面が
自分で期限を見ると、取り直しの処理が散らばって必ずどこかが漏れる。
`APIClient` には値ではなく**関数**を渡し、呼び出しのたびに解決する
（作った時点の値を握ると1時間後から 401 になる）。

リフレッシュトークンは **Keychain** に置く。持っている限りアクセストークンを
取り直せるので、`UserDefaults` のようにバックアップから平文で読める場所には置かない。

## 画面

| タブ | 何をするか | 要件 |
|---|---|---|
| 記録 | トレーニングのセット記録。オフライン対応 | T-01〜T-07 |
| 食事 | 食品名・量・PFC・kcal。過去の記録から選べる | N-01 / N-02 / N-05 |
| 体組成 | 体重・体脂肪率・周囲長・疲労度。HealthKit 連携 | B-01〜B-03 / B-06 |
| 設定 | ログアウトのみ。**設定そのものは Web で触る** | — |

**食事は「過去の記録から選ぶ」が主経路。** 食品マスタを持たない設計なので
（要件 N-02）、記録がそのままマスタになる。選んだ時点で入力が終わる。

目標（残量）は出せないことがある。フェーズ未登録だと API が 422 を返すためで、
**そのときも記録はできる**。理由だけ画面に出す。

## テストとビルド

```bash
swift test                 # ロジック。速いので普段はこれ
./scripts/device.sh        # 実機にビルドして入れて起動する
```

開発の流れ（シミュレータを含む）は [docs/swift/dev-flow.md](../docs/swift/dev-flow.md)。

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
