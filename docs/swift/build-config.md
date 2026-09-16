# 接続先がアプリに届くまで（xcconfig → Info.plist → コード）

**iOS には環境変数が無い。** サーバなら `os.Getenv` で済むものを、
ビルド時に焼き込む必要がある。

## 経路

```
Physique/Base.xcconfig          commit する。既定値
        │  #include? "Config.xcconfig"
        ▼
Physique/Config.xcconfig        .gitignore。実値
        │  （xcodeproj の baseConfigurationReference から読まれる）
        ▼
ビルド設定 API_BASE_URL = https://...
        │  Info.plist の $(API_BASE_URL) が展開される
        ▼
Physique.app/Info.plist         焼き込まれた値
        │  Bundle.main.object(forInfoDictionaryKey:)
        ▼
AppConfig.apiBaseURL
```

## 1. xcconfig

```
// Base.xcconfig（commit する）
PRODUCT_BUNDLE_IDENTIFIER = io.github.kenyan822.physique

API_BASE_URL = http:/$()/localhost:8080
SUPABASE_URL =
SUPABASE_ANON_KEY =
DEVELOPMENT_TEAM =

#include? "Config.xcconfig"
```

### `//` を書けない

**xcconfig は `//` 以降をコメントとして落とす。** URL をそのまま書くと

```
API_BASE_URL = https://example.com     →  https:
```

になる。`$()`（空の変数展開）を挟んで分断する。

```
API_BASE_URL = https:/$()/example.com
```

ビルド設定としては `https://example.com` になる。

### `#include?` の `?`

`?` なしの `#include` は、ファイルが無いとビルド設定の読み込みが失敗する。
`?` 付きなら黙って飛ばす。

**クローンしただけの状態でもビルドが通る**ようにするため。
既定値（`localhost:8080`）で動く。

### 後から読む方が勝つ

xcconfig は**上から順に代入**していくだけ。`#include?` を末尾に置けば、
`Config.xcconfig` の値が `Base.xcconfig` の既定値を上書きする。

## 2. プロジェクトへの接続

`project.pbxproj` の**プロジェクトレベル**の `XCBuildConfiguration` に
`baseConfigurationReference` を置いている。

```
A1000001000000000000070 /* Debug */ = {
    isa = XCBuildConfiguration;
    baseConfigurationReference = A1000001000000000000003 /* Base.xcconfig */;
    buildSettings = { ... };
};
```

Xcode の `PROJECT > Info > Configurations` で設定するのと同じことを、
**ファイルに直接書いてコミットしてある**。GUI 操作を手順から消すため。

### 優先順位が重要

```
ターゲットの明示設定  >  プロジェクトの明示設定  >  xcconfig  >  Xcode の既定
```

**xcconfig はいちばん弱い。** だから `PRODUCT_BUNDLE_IDENTIFIER` を
ターゲットの `buildSettings` から削除してある。残したままだと
xcconfig 側の値が無視される。

`xcodebuild -showBuildSettings` で解決後の値を確認できる。

```console
$ xcodebuild -project Physique.xcodeproj -target Physique -showBuildSettings \
  | grep -E "API_BASE_URL|PRODUCT_BUNDLE_IDENTIFIER"
API_BASE_URL = https://physique-api-....run.app
PRODUCT_BUNDLE_IDENTIFIER = io.github.kenyan822.physique
```

## 3. Info.plist に展開する

```xml
<!-- 接続先と鍵。**ビルド設定（xcconfig / Build Settings）から差し込む。**
     ここに実値を書くと public リポジトリに入る。 -->
<key>API_BASE_URL</key>
<string>$(API_BASE_URL)</string>
<key>SUPABASE_URL</key>
<string>$(SUPABASE_URL)</string>
<key>SUPABASE_ANON_KEY</key>
<string>$(SUPABASE_ANON_KEY)</string>
```

ビルド時に `ProcessInfoPlistFile` フェーズが `$(...)` を展開する。
`-expandbuildsettings` が付いているのがそれ。

焼き込まれた結果はアプリバンドルから読める。

```console
$ plutil -p build/Debug-iphonesimulator/Physique.app/Info.plist | grep API_BASE_URL
  "API_BASE_URL" => "https://physique-api-....run.app"
```

## 4. コードから読む

```swift
enum AppConfig {
    static var apiBaseURL: URL {
        url(for: "API_BASE_URL") ?? URL(string: "http://localhost:8080")!
    }

    static var supabaseURL: URL? { url(for: "SUPABASE_URL") }
    static var supabaseAnonKey: String? { string(for: "SUPABASE_ANON_KEY") }

    private static func string(for key: String) -> String? {
        guard let s = Bundle.main.object(forInfoDictionaryKey: key) as? String,
              !s.isEmpty else { return nil }

        return s
    }

    private static func url(for key: String) -> URL? {
        string(for: key).flatMap(URL.init(string:))
    }
}
```

**空文字列を nil として扱う。** `Base.xcconfig` の `SUPABASE_URL =` は
空文字列として展開されるので、`!s.isEmpty` が無いと
「設定されている空の URL」になる。

`enum` にして `case` を持たせないのは、**インスタンス化を型で禁じる**イディオム。
`struct` だと `AppConfig()` が書けてしまう。

## Info.plist に鍵を入れることについて

**`Info.plist` はアプリバンドルから誰でも読める。** `.ipa` を展開すれば見える。

入れているのは

| | 秘密か |
|---|---|
| `API_BASE_URL` | いいえ。URL は隠せない |
| `SUPABASE_URL` | いいえ |
| `SUPABASE_ANON_KEY` | **いいえ。ブラウザの JS にも載る公開鍵** |

anon key 単体では何も読めない。PostgREST は RLS で閉じ、Go API は
`ALLOWED_USER_IDS` で弾く（[auth.md](auth.md)）。

**秘密にすべきものはここに置かない。** リフレッシュトークンは Keychain。

## 署名

```
Config.xcconfig
    DEVELOPMENT_TEAM = XXXXXXXXXX
        │
        ▼
CODE_SIGN_STYLE = Automatic （ターゲットの設定）
        │
        ▼
Xcode が App ID を登録し、provisioning profile を生成
```

### bundle ID を `com.example.*` にしない

無料の Personal Team でも、Xcode は**実際に Apple のサーバに App ID を登録する**。
`com.example.physique` は他人が押さえている可能性が高く、

```
Failed to register bundle identifier.
The app identifier cannot be registered to your development team because it is not available.
```

で止まる。リポジトリの所有者から引いた一意な値にしてある。

### DEVELOPMENT_TEAM を xcconfig に置く理由

Xcode の `Signing & Capabilities` で Team を選ぶと、
**`project.pbxproj` に `DEVELOPMENT_TEAM = XXXXXXXXXX;` が書き込まれる**。
これは commit 対象のファイルなので、個人の Team ID が公開リポジトリに入る。

xcconfig（gitignore）から渡せば入らない。

### Personal Team の制約

| | |
|---|---|
| 有効期限 | **7日**。切れたら再インストール |
| デバイス登録 | **実機を繋ぐまで profile を作れない**（`Your team has no devices` が出る） |
| HealthKit | **entitlement を付けられない** |
| Push 通知 | 不可 |

HealthKit が使えないので、`BodyModel.syncHealth` は実行時に失敗する。
握って理由を出すだけにしてあり、他の機能は動く。

```swift
do {
    try await health.requestAuthorization()
    ...
} catch {
    message = ...
}
```

## macOS 側のビルド（`swift test`）はここを通らない

`Package.swift` は `Info.plist` を `exclude` している。
`AppConfig` を使うコードは `LogModel.init` の既定引数にあるが、
テストでは `APIClient` を明示的に渡すので評価されない。

```swift
init(
    api: APIClient = APIClient(baseURL: AppConfig.apiBaseURL),   // ← 既定引数
    ...
)
```

`Bundle.main` は `swift test` だとテストバンドルを指し、キーが無いので
`nil` → `localhost:8080` にフォールバックする。テストは `FakeTransport` を
使うので実際には繋がない。
