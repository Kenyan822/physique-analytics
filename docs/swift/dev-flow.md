# 直したあと何をするか

Swift を変更してから、動いていることを確かめてマージするまで。

**時間の桁が3段階ある。** 速い順に試すのが基本。

| | 何を見るか | 実測 |
|---|---|---|
| `swift test` | ロジック。**View は見ない** | **約10秒** |
| シミュレータ | 画面を含むコンパイル＋見た目 | 差分 **18秒** / クリーン 53秒 |
| 実機 | 実際の通信・Keychain・タッチ | 差分ビルド **4秒** ＋ インストール数秒 |

## 1. 内側のループ

```bash
cd ios
swift test
```

93件が0.02秒で走る（残りはビルドの確認）。**大半の変更はここで終わる。**

判断を `*Model.swift` に追い出してあるので、ロジックの修正はほぼこれで検証できる
（[app-structure.md](app-structure.md#この分け方の代償)）。

### `swift test` は View を見ていない

`Package.swift` が `exclude` している。

```swift
exclude: [
    "Info.plist", "PhysiqueApp.swift",
    "LogView.swift", "BodyView.swift", "HealthKitSource.swift",
    "LoginView.swift", "MealView.swift",
]
```

| 直した場所 | 次にやること |
|---|---|
| `*Model.swift` / `*Models.swift` / `APIClient` / `JST` など | `swift test` で足りる |
| **`*View.swift` / `PhysiqueApp.swift`** | **ビルドしないと型エラーに気づけない** |

タイムゾーンに依存するコードを触ったら、`TZ` を変えて回す（[testing.md](testing.md#踏んだ罠)）。

```bash
TZ=UTC swift test
```

## 2. シミュレータ

画面を直したとき、実機を繋がずに見たいとき。

```bash
cd ios
SIM="iPhone 17"

# ビルド
xcodebuild -project Physique.xcodeproj -scheme Physique \
  -destination "platform=iOS Simulator,name=$SIM" \
  -derivedDataPath /tmp/dd-sim build

# 起動してアプリを入れる
xcrun simctl boot "$SIM" 2>/dev/null
open -a Simulator
xcrun simctl bootstatus "$SIM" -b
xcrun simctl install "$SIM" /tmp/dd-sim/Build/Products/Debug-iphonesimulator/Physique.app
xcrun simctl launch "$SIM" io.github.kenyan822.physique
```

`simctl` は**機種名で指定できる**（UDID でなくてよい）。使える名前は

```bash
xcrun simctl list devices available
```

### 起動し切る前に `install` すると失敗する

```
Simulator device failed to launch io.github.kenyan822.physique.
No such process
```

`boot` は**非同期で戻る**。`bootstatus <device> -b` を挟むと起動完了まで待つ。

### 画面を見る

```bash
xcrun simctl io "iPhone 17" screenshot /tmp/sim.png
xcrun simctl ui "iPhone 17" appearance dark      # ダークモード
```

**レイアウト崩れはここで潰す。** `swift test` は View を見ないので、
「型は通るが表示が変」はスクリーンショットでしか分からない。

#### シミュレータは既定でソフトウェアキーボードが出ない

Mac のキーボードに繋がるため。`.onAppear { focus = .email }` のように
起動直後にフォーカスを当てる画面は、**実機と見え方が変わる**。

キーボードが出た状態のレイアウトを確かめたいときは、
Simulator の `I/O > Keyboard > Connect Hardware Keyboard` を切る。

### ログを見る

`print` は Xcode を通さないと見えない。`os.Logger` を使えば `simctl` から拾える。

```bash
xcrun simctl spawn "iPhone 17" log stream --level debug \
  --predicate 'subsystem contains "physique"' --style compact
```

### おかしくなったら消す

```bash
xcrun simctl uninstall "iPhone 17" io.github.kenyan822.physique   # アプリだけ
xcrun simctl erase "iPhone 17"                                     # まっさら
```

`UserDefaults`（`healthLastSynced`）や送信待ちキューの `pending.json` は
アプリの中に残る。**初回起動の挙動を確かめるときは `uninstall` する。**

### シミュレータで確かめられないこと

| | なぜ |
|---|---|
| **HealthKit** | データが空。取り込みの判断は `HealthSync` のテストで見る |
| **Keychain の保護クラス** | `AfterFirstUnlock` の意味が無い |
| **圏外・電波が悪い状態** | 送信待ちキューの本番は実機 |
| **通知** | 許可ダイアログの挙動が違う |

## 3. 実機

**Xcode を開かなくてよい。1コマンドで入る。**

```bash
cd ios
./scripts/device.sh              # ビルド → インストール → 起動
./scripts/device.sh --no-launch  # 入れるだけ
```

**デバイスは自動検出する。** 端末名も UDID も打たない（公開リポジトリに
個人の端末が特定できる情報を置かないため）。繋がっていない・複数繋がっている・
デベロッパモードが無効、はそれぞれ対処つきで止まる。

中でやっているのはこの2つ。

```bash
xcodebuild -project Physique.xcodeproj -scheme Physique \
  -destination "platform=iOS,name=$DEVICE" \
  -derivedDataPath "$DERIVED" -allowProvisioningUpdates build

xcrun devicectl device install app --device "$DEVICE" \
  "$DERIVED/Build/Products/Debug-iphoneos/Physique.app"
```

**UDID を調べる必要は無い。** `xcodebuild -destination` も `devicectl --device` も
**デバイス名を受け付ける**。名前は `xcrun devicectl list devices` の `Name` 列
（iPhone の `設定 > 一般 > 情報 > 名前`）。

`-allowProvisioningUpdates` が要るのは、**デバイスを Personal Team に登録して
provisioning profile を作らせる**ため。付けないと初回に
`no devices ... to generate a provisioning profile` で止まる。

**2回目以降は証明書の信頼もデベロッパモードも要らない。** 同じ証明書なので
入れ替えたらすぐ起動する。

初回の手順（デベロッパモード・証明書の信頼）は
[ios/README.md](../../ios/README.md#実機で動かす)。

### 起動まで自動でやる

```bash
xcrun devicectl device process launch --device "$DEVICE" io.github.kenyan822.physique
```

証明書を信頼していないと

```
Unable to launch ... its profile has not been explicitly trusted by the user
```

で断られる。初回だけ iPhone 側で信頼する。

### 状態を調べる

```bash
xcrun devicectl list devices
xcrun devicectl device info details --device "$DEVICE" \
  | grep -iE "osVersion|developerMode|pairingState|tunnelState"
```

`developerModeStatus` / `pairingState` / `tunnelState` が読める。
**「繋がっているのに Xcode から見えない」を切り分けるのはこれが速い。**

### 7日で切れる

無料の Personal Team の provisioning profile の期限。起動しなくなったら
上の2コマンドを打ち直す。**記録は消えない**（データは Cloud Run の先）。

## 4. 外側のループ

[CLAUDE.md](../../CLAUDE.md) の運用どおり。

```
issue  →  ブランチ  →  テストを先に  →  実装  →  swift test
   →  シミュレータ / 実機で確認  →  PR  →  CI 緑  →  マージ
```

CI（`ios (Swift)`）がやるのは2つ。

1. `swift test`
2. シミュレータ向けの `xcodebuild build` —— **View のコンパイルはここで拾われる**

手元でシミュレータビルドを回していれば、CI で初めて気づくことはない。

## 5. デプロイに相当するものが無い

| | 反映のされ方 |
|---|---|
| Web | `vercel deploy --prod`（Git 未接続。[#173](https://github.com/Kenyan822/physique-analytics/issues/173)） |
| API | main マージで `deploy-api` が自動。マイグレーションも自動（[ADR-0016](../adr/0016-auto-migrate-on-deploy.md)） |
| **iOS** | **無い。手元で再インストールする** |

**マージしても iPhone のアプリは変わらない。** TestFlight も未設定なので、
配布の仕組みそのものが無い。利用者が1人なので当面これで足りる。

## 6. `openapi.yaml` を変えたとき

**Swift だけコード生成していない**（[codegen.md](../go/codegen.md#他の言語との対応)）。

```
openapi.yaml を編集
  ├─ Go:    cd api && go generate ./...    自動
  ├─ TS:    cd web && pnpm gen             自動
  └─ Swift: 手で直す                        ←
```

`Models.swift` / `MealModels.swift` / `BodyModels.swift` と `APIClient.swift` を
手で追随させる。enum の値がずれたらテストが落ちる。

```swift
@Test("openapi.yaml の値と一致する")
func rawValues() {
    #expect(MealSlot.breakfast.rawValue == "朝食")
}
```

**フィールドの追加漏れは落ちない。** そこは目で見るしかないのが今の弱点。

## 7. 学びを残す

Go と同じ扱い。Swift で学んだことは `docs/swift/<トピック>.md` に足して、
[README.md](README.md) の索引に載せる。
