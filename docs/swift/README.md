# Swift の学び

iOS クライアントの**仕組み**と、Swift で学んだこと。
Go 側は [docs/go/](../go/)。

## 何を書くか

`docs/go/README.md` と同じ方針。**コードを読めば分かることは書かない。**

書くのは次の3つ。

1. **層と層の間で何が起きているか** —— コードを縦に読まないと見えない動線
2. **なぜその形なのか** —— 選ばなかった方も含めて
3. **Swift / SwiftUI 固有の落とし穴** —— 他言語の勘が効かないところ

## 索引

| トピック | 内容 |
|---|---|
| [dev-flow.md](dev-flow.md) | **直したあと何をするか**。`swift test` → シミュレータ → 実機 → PR。実測つき |
| [app-structure.md](app-structure.md) | **ios/ の構成**。SPM と `.xcodeproj` の二重ビルド、依存の向き、`exclude` の理由 |
| [observation.md](observation.md) | `@Observable` / `@State` / `@Bindable`。**どうして画面が描き直されるのか** |
| [concurrency.md](concurrency.md) | `async`/`await`、`@MainActor`、`Sendable`。コンパイラがデータ競合を落とす仕組み |
| [networking.md](networking.md) | `APIClient` の内部。ジェネリクス、204、`problem+json` の受け方 |
| [auth.md](auth.md) | **起動からトークンが `Authorization` に載るまで**。Keychain と自動更新 |
| [offline.md](offline.md) | `PendingQueue`。電波が切れても記録が消えない仕組み（要件 T-07） |
| [build-config.md](build-config.md) | xcconfig → `Info.plist` → `AppConfig`。署名と bundle ID |
| [testing.md](testing.md) | Swift Testing、偽物の注入、タイムゾーン依存の落とし方 |

## 全体像

```
                    PhysiqueApp（@main）
                          │
             AuthModel.isSignedIn で分岐
                    ┌─────┴─────┐
              LoginView      MainTabs
                                │
                    ┌───────┬───┴───┬────────┐
                 LogView  MealView BodyView SettingsView
                    │        │        │
                 LogModel MealModel BodyModel     ← @Observable @MainActor
                    │        │        │
                    └────────┼────────┘
                             │
                        APIClient                  ← struct / Sendable
                             │
                   ┌─────────┴─────────┐
              tokenProvider        HTTPTransport
                   │                    │
               AuthModel            URLSession
                   │
        AuthClient ─┴─ SessionStore（Keychain）
```

**画面 → モデル → APIClient** の一方向。モデルは `View` を知らない。

`LogModel` だけ `PendingQueue` を持つ（[offline.md](offline.md)）。
ジムで電波が切れる前提があるのは記録画面だけで、他は失敗したら
メッセージを出して終わりにしている。
