# 起動してからトークンが `Authorization` に載るまで

ログインの仕組み。**SDK を入れていない**理由と、トークンが切れたときに
何が起きるか。

## なぜ supabase-swift を入れなかったか

使うのは3つだけ。

| | エンドポイント |
|---|---|
| ログイン | `POST /auth/v1/token?grant_type=password` |
| 取り直し | `POST /auth/v1/token?grant_type=refresh_token` |
| ログアウト | `POST /auth/v1/logout` |

アプリは既に `HTTPTransport`（[networking.md](networking.md)）を持っている。
依存を1つ増やすより既存の層に載せる方が、

- **テストが同じ仕組みで書ける**（`FakeTransport` を使い回せる）
- `.xcodeproj` に SPM 依存を手で足さずに済む
- 認証の挙動が全部この2ファイルに見える

エンドポイントが増えたら見直す。

## 登場人物

```
AuthClient      Supabase を叩くだけ。状態を持たない struct
AuthModel       @Observable @MainActor。セッションを持つ唯一の場所
SessionStore    protocol。実装は KeychainSessionStore
Session         accessToken / refreshToken / expiresAt
```

## 起動時 —— Keychain から復元する

```swift
// PhysiqueApp.swift
@State private var auth = AuthModel(
    auth: AuthClient(
        url: AppConfig.supabaseURL ?? URL(string: "https://supabase-url-未設定.invalid")!,
        anonKey: AppConfig.supabaseAnonKey ?? ""
    ),
    store: KeychainSessionStore()
)

var body: some Scene {
    WindowGroup {
        if auth.isSignedIn { MainTabs(auth: auth) } else { LoginView(auth: auth) }
    }
}
```

```swift
// AuthModel.init
self.session = store.load()     // ← 同期。ここで isSignedIn が決まる
```

**`init` の中で Keychain から読む。** 非同期にすると起動直後に一瞬
`LoginView` が出てから切り替わる。Keychain の読み取りは十分速い。

### 未設定の URL をわざと壊す

```swift
URL(string: "https://supabase-url-未設定.invalid")!
```

`AppConfig.supabaseURL` が nil（= xcconfig 未設定）のときのフォールバック。
`localhost` にすると「なぜか繋がらない」になるが、`.invalid` なら
**DNS で即座に失敗する**ので設定漏れだと分かる。

`.invalid` は RFC 2606 で予約された「絶対に解決されない」TLD。

## ログイン

```swift
func signIn(email: String, password: String) async {
    errorMessage = nil
    isWorking = true
    defer { isWorking = false }

    do {
        let s = try await auth.signIn(email: email, password: password)
        store.save(s)
        session = s               // ← @Observable。ここで画面が MainTabs に切り替わる
    } catch {
        errorMessage = (error as? LocalizedError)?.errorDescription ?? "ログインできない"
    }
}
```

`session` に代入した瞬間、`isSignedIn` を読んでいる `PhysiqueApp.body` が
再評価されて `MainTabs` に差し替わる（[observation.md](observation.md)）。

### `apikey` ヘッダが要る

```swift
var req = URLRequest(url: components.url!)
req.httpMethod = "POST"
// **apikey を付けないと Supabase が受け付けない**
req.setValue(anonKey, forHTTPHeaderField: "apikey")
req.setValue("application/json", forHTTPHeaderField: "Content-Type")
```

Supabase のゲートウェイ（Kong）が `apikey` でプロジェクトを判別する。
無いと 401 になり、資格情報の誤りと区別がつかない。

anon key は**公開前提**。ブラウザの JS に載るもので、単体では何も読めない
（DB は RLS で閉じている）。

### 期限は「秒数」ではなく「時刻」で持つ

```swift
return Session(
    accessToken: res.access_token,
    refreshToken: res.refresh_token,
    expiresAt: now.addingTimeInterval(TimeInterval(res.expires_in))
)
```

Supabase は `expires_in: 3600`（秒）を返す。**そのまま保存しない。**
秒数のままだと、Keychain から読むたびに「いつ取得したか」と
突き合わせることになる。取得時刻を足して `Date` にしてしまう。

`now` を引数にしてあるのでテストで固定できる。

## トークンが API に載るまで

```swift
// PhysiqueApp.swift
private var api: APIClient {
    APIClient(
        baseURL: AppConfig.apiBaseURL,
        tokenProvider: { [auth] in try await auth.accessToken() }
    )
}
```

**値ではなくクロージャを渡す。** `APIClient` は `AuthModel` を知らないまま、
リクエストのたびに最新のトークンを受け取る。

```
APIClient.sendRequest
    └─ try await tokenProvider?()
          └─ AuthModel.accessToken()
                ├─ 期限内 → そのまま返す
                └─ 期限が近い → AuthClient.refresh → Keychain に保存 → 返す
```

### 期限の判定に 60 秒の余裕を入れる

```swift
func isUsable(at now: Date = Date()) -> Bool {
    expiresAt.timeIntervalSince(now) > Self.margin
}

/// 期限までの余裕。往復と時計のずれを見込む
static let margin: TimeInterval = 60
```

ちょうどの判定だと、**送信中に切れて 401 になる**。
サーバとの時計のずれもあるので先に取り直す。

### 取り直しに失敗したらログアウトする

```swift
func accessToken() async throws -> String {
    guard let current = session else { throw AuthError.needsSignIn }
    if current.isUsable() { return current.accessToken }

    do {
        let renewed = try await auth.refresh(refreshToken: current.refreshToken)
        store.save(renewed)
        session = renewed
        return renewed.accessToken
    } catch {
        store.clear()
        session = nil            // ← LoginView に戻る
        throw AuthError.needsSignIn
    }
}
```

**保存を残さない。** 残すと、起動するたびに `isSignedIn == true` で
`MainTabs` を出しては全画面が 401 になる、という状態から抜けられない。

`session = nil` にすると `PhysiqueApp.body` が再評価されて
`LoginView` に戻る。画面側に「ログアウト処理」を書かなくてよい。

### 400 の意味が grant によって違う

```swift
if http.statusCode == 400 {
    throw grant == "password" ? AuthError.invalidCredentials : AuthError.needsSignIn
}
```

| grant | 400 の意味 |
|---|---|
| `password` | メールかパスワードが違う |
| `refresh_token` | リフレッシュトークンが失効した |

前者は「入力し直して」、後者は「ログインし直して」。同じ 400 でも
ユーザーがやることが違うので、ここで分ける。

**どちらが違うかは区別しない。** 「このメールは存在する」を教えると
アカウントの存在を確認する手段になる。

## Keychain に置く理由

```swift
/// **UserDefaults に置かない。** リフレッシュトークンは、持っている限り
/// アクセストークンを取り直せる。端末のバックアップから平文で読める場所に
/// 置くものではない。
struct KeychainSessionStore: SessionStore {
```

`UserDefaults` は `~/Library/Preferences/*.plist` に平文で入る。
**リフレッシュトークンは実質的に無期限のパスワード**なので置けない。

### 保護クラス

```swift
q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
```

| | いつ読めるか |
|---|---|
| `WhenUnlocked` | 画面ロック解除中だけ |
| `AfterFirstUnlock` | **再起動後、一度解除すればロック中も** |
| `Always` | 常に（非推奨） |

`AfterFirstUnlock` にしているのは、将来バックグラウンド同期を入れたときに
ロック中でも動けるようにするため。

### 更新は delete → add

```swift
// 上書きは delete → add。SecItemUpdate だと不在時に失敗する
SecItemDelete(query as CFDictionary)

var q = query
q[kSecValueData as String] = data
q[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlock
SecItemAdd(q as CFDictionary, nil)
```

`SecItemUpdate` は項目が無いと `errSecItemNotFound` を返す。
初回ログインで必ず失敗するので、**常に消してから足す**。

`SecItemDelete` は不在でもエラーを返すだけで害が無いので、戻り値を見ていない。

## ログアウトは失敗を握りつぶす

```swift
/// ログアウト。**失敗しても握りつぶす。**
/// 手元のセッションを捨てる方が本質で、サーバ側の失効は届けば良い
func signOut(accessToken: String) async {
    ...
    _ = try? await transport.send(req)
}
```

圏外でログアウトを押したときに「ログアウトできません」と出すのは意味が無い。
**手元の Keychain を消せば、そのトークンはもう使われない。**

## サーバ側との対応

iOS が送った JWT を Go API がどう検証するかは
[docs/go/request-flow.md](../go/request-flow.md#2-認証ミドルウェア) を参照。

要点だけ:

- 署名の検証は「Supabase が発行したか」しか見ない
- **誰のトークンかは `ALLOWED_USER_IDS` で別途絞っている**（#51 / #142）
- 未設定なら全員 401（fail-closed、#152）
