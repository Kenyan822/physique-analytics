# 構成と、二重にビルドする理由

`ios/` の中身と、**なぜ SPM と `.xcodeproj` の両方があるのか**。

## 全体

```
ios/
├── Package.swift              SPM。swift test 用
├── Physique.xcodeproj/        Xcode。実機・シミュレータ用
├── Physique/
│   ├── Base.xcconfig          ビルド設定の入口（commit する）
│   ├── Config.xcconfig        接続先と鍵（.gitignore）
│   └── Sources/
│       ├── Info.plist
│       ├── PhysiqueApp.swift  @main
│       ├── *View.swift        SwiftUI
│       ├── *Model.swift       画面の状態（@Observable）
│       ├── *Models.swift      値型
│       ├── APIClient.swift    HTTP
│       ├── AuthClient.swift   Supabase Auth
│       ├── SessionStore.swift Keychain
│       ├── PendingQueue.swift 送信待ち
│       ├── HealthKitSource.swift
│       └── JST.swift
└── PhysiqueTests/             Swift Testing
```

## 同じソースを2つのビルドシステムが見ている

**`Package.swift` と `.xcodeproj` は、どちらも `Physique/Sources` を指している。**
ソースの複製はしていない。

```swift
.target(
    name: "PhysiqueCore",
    path: "Physique/Sources",
    exclude: [
        "Info.plist", "PhysiqueApp.swift",
        // UI と HealthKit は iOS 専用。macOS ビルドで落ちる
        "LogView.swift", "BodyView.swift", "HealthKitSource.swift",
        "LoginView.swift", "MealView.swift",
    ]
)
```

### なぜ分けるか

| | 何ができるか | 所要 |
|---|---|---|
| `swift test` | ロジックのテスト。**Xcode を起動しない** | 1秒未満 |
| `xcodebuild` | UI を含む全ファイルの型チェック、実機へのインストール | 分単位 |

**`swift test` は macOS ネイティブで走る。** シミュレータを起動しないので速い。
CI もこの順で、まず `swift test`、通ったら `xcodebuild` でコンパイルを見る。

### `exclude` に何が入るか

**macOS でコンパイルできないものだけ。** 判定は「iOS 専用 API を使っているか」。

```swift
.keyboardType(.emailAddress)        // UIKit 由来。macOS に無い
.textInputAutocapitalization(.never)
import HealthKit                    // macOS にもあるが型が違う
```

`HealthKitSource.swift` は `#if canImport(HealthKit)` で囲ってあるが、
**それでも `exclude` している**。`canImport` は「import できるか」しか見ないので、
macOS でも通ってしまい、中の iOS 専用 API で落ちる。

### この分け方の代償

**UI のロジックは `swift test` で触れない。** だから
「画面の状態」を `*Model.swift` に追い出してある。

```swift
// MealView.swift —— 画面は model を読むだけ
Total(label: "kcal", value: model.totals.kcal, remaining: model.remaining?.kcal)
```

`totals` と `remaining` は `MealModel` の計算プロパティで、
`MealModelTests` が直接叩ける。View には**判断を置かない**。

## 依存の向き

```
PhysiqueApp
    │
    ├──▶ AuthModel ──▶ AuthClient ──▶ HTTPTransport
    │         └──────▶ SessionStore（Keychain）
    │
    └──▶ MainTabs
            │
            ├──▶ LogView  ──▶ LogModel  ──▶ APIClient
            │                      └─────▶ PendingQueue ──▶ PendingStore
            ├──▶ MealView ──▶ MealModel ──▶ APIClient
            └──▶ BodyView ──▶ BodyModel ──▶ APIClient
                                   └─────▶ HealthSource
```

**下は上を知らない。** `APIClient` は `AuthModel` を import しない
——クロージャ（`TokenProvider`）を受け取るだけ（[auth.md](auth.md)）。

Go と違い、Swift は**同一モジュール内なら循環 import が起きない**
（ファイル単位の import が無い）。構造として守られないので、
「モデルが View を参照していないか」は目で見るしかない。

## ファイルの分け方

| 接尾辞 | 中身 |
|---|---|
| `*View.swift` | SwiftUI。`body` と小さな部品。**判断を書かない** |
| `*Model.swift` | `@Observable @MainActor final class`。画面の状態と手続き |
| `*Models.swift` | `struct` / `enum`。値型だけ。`Codable` はここ |
| `*Client.swift` | 外部との通信。`struct` で `Sendable` |
| `*Store.swift` | 永続化。`protocol` + 実装 |

**`Model` と `Models` の1文字差は読みにくい**が、
`MealModel`（状態）と `MealModels`（`Meal` / `MealInput` / `MealSlot`）は
役割がはっきり違うので分けている。

## 生成コードが無い

Web と Go は `openapi.yaml` から型を生成している（[ADR-0007](../adr/0007-openapi-schema-driven.md)）が、
**Swift は手書き**。

```swift
/// 型は `openapi.yaml` が正（ADR-0007）。Swift では
/// swift-openapi-generator を使う方針だが、まずは手書きの薄い層で動かし、
/// エンドポイントが増えてから生成に切り替える。
```

いまの `APIClient` は 281 行で、生成器（と生成物、CI の最新性チェック）を
入れるコストに見合っていない。**`openapi.yaml` が正である点は変わらない**ので、
ずれたら `swift test` の `"openapi.yaml の値と一致する"` が落ちる。

```swift
@Test("openapi.yaml の値と一致する")
func rawValues() {
    #expect(MealSlot.breakfast.rawValue == "朝食")
    ...
}
```
