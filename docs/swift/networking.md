# `APIClient` の中で起きていること

281 行。`GET /v1/exercises` を呼ぶと何が起きるかを上から下まで追う。

## 3層になっている

```
listExercises()          ← 公開メソッド。型と URL を決める
    │
request(Response.self, "GET", "/v1/exercises", query:)
    │                    ← JSON をデコードする
sendRequest("GET", ...)
    │                    ← URL 組み立て・認証・ステータス判定
transport.send(req)
    │                    ← URLSession（テストでは偽物）
  (Data, HTTPURLResponse)
```

**`request` と `sendRequest` が分かれているのが肝。** 理由は後述（204）。

## 1. 公開メソッド —— 応答の形をその場で定義する

```swift
func listExercises(muscleGroup: MuscleGroup? = nil) async throws -> [Exercise] {
    var query: [URLQueryItem] = []
    if let mg = muscleGroup {
        query.append(URLQueryItem(name: "muscleGroup", value: mg.rawValue))
    }

    struct Response: Decodable { let items: [Exercise] }

    return try await request(Response.self, "GET", "/v1/exercises", query: query).items
}
```

`struct Response` を**関数の中で宣言している**。API は `{"items": [...]}` を返すが、
呼ぶ側が欲しいのは `[Exercise]` だけ。包みの型に名前を付けて
ファイルスコープに置くと、`ExercisesResponse` `MealsResponse` … が増える。

Swift は**関数内で型を宣言できる**ので、使う場所に閉じ込められる。

## 2. `request` —— ジェネリクスでデコード先を受け取る

```swift
private func request<T: Decodable>(
    _ type: T.Type,
    _ method: String,
    _ path: String,
    query: [URLQueryItem] = [],
    body: (some Encodable)? = Optional<String>.none
) async throws -> T {
    let data = try await sendRequest(method, path, query: query, body: body)

    do {
        return try JSONDecoder().decode(T.self, from: data)
    } catch {
        throw APIError.decoding(error)
    }
}
```

### `_ type: T.Type` を渡す理由

戻り値の型 `T` は、**呼び出し側の文脈からは決まらないことがある**。

```swift
return try await request(Response.self, ...).items   // .items を取るので T が推論できない
```

`Response.self`（メタタイプ）を引数で渡すと `T` が確定する。
Swift の標準ライブラリも `decode(T.self, from:)` で同じ形をとっている。

### `body: (some Encodable)?` は「不透明な引数型」

`some Encodable` を引数に書くと、`<U: Encodable>(body: U?)` と同じ意味になる
（Swift 5.7 以降の糖衣）。ジェネリックパラメータを1つ増やさずに書ける。

既定値が `Optional<String>.none` なのは、**`nil` だけでは `U` が決まらない**ため。
「`String` の nil」と具体化して型検査を通している。

## 3. `sendRequest` —— ここが本体

```swift
private func sendRequest(
    _ method: String, _ path: String,
    query: [URLQueryItem], body: (some Encodable)?
) async throws -> Data {
```

### URL を組み立てる

```swift
var components = URLComponents(
    url: baseURL.appendingPathComponent(path), resolvingAgainstBaseURL: false
)
if !query.isEmpty { components?.queryItems = query }
guard let url = components?.url else { throw APIError.transport(URLError(.badURL)) }
```

**`URLComponents` を使うのはエスケープのため。** `queryItems` に入れた値は
自動でパーセントエンコードされる。日本語の部位名（`muscleGroup=胸`）を
文字列連結で組むと壊れる。

テストで確認している。

```swift
@Test("部位で絞ると日本語がエンコードされる")
```

### トークンを「その場で」解決する

```swift
if let token = try await tokenProvider?() {
    req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
}
```

**値ではなく関数を持っている。** アクセストークンは1時間で切れるので、
`APIClient` を作った時点の文字列を握ると1時間後から全部 401 になる。

```swift
typealias TokenProvider = @Sendable () async throws -> String?
```

`nil` を返せばヘッダを付けない。認証が要らない経路（`/health`）と
テストで使う。詳細は [auth.md](auth.md)。

### 本文を付けるのは `body` があるときだけ

```swift
if let body {
    req.setValue("application/json", forHTTPHeaderField: "Content-Type")
    req.httpBody = try JSONEncoder().encode(body)
}
```

`if let body` は `if let body = body` の省略形（Swift 5.7 以降）。

GET に `Content-Type` を付けないのは礼儀の問題ではなく、
**プリフライトや中間装置の挙動を無駄に変えない**ため。

### エラーの二重包みを避ける

```swift
do {
    (data, http) = try await transport.send(req)
} catch let e as APIError {
    throw e                              // ← すでに APIError なら包み直さない
} catch {
    throw APIError.transport(error)
}
```

`URLSessionTransport` は `APIError.transport` を投げることがある。
無条件に包むと `APIError.transport(APIError.transport(...))` になり、
`errorDescription` が「通信できない: 通信できない: ...」になる。

### ステータス判定と `problem+json`

```swift
guard (200..<300).contains(http.statusCode) else {
    // problem+json で返ってこないこともある（LB の 502 など）。
    // そこで落ちるとエラーの原因が「解釈できない」にすり替わる
    let problem = try? JSONDecoder().decode(Problem.self, from: data)
    throw APIError.http(status: http.statusCode, problem: problem)
}
```

**`try?` なのが重要。** Go API は RFC 7807 の `problem+json` を返すが、
Cloud Run のロードバランサが返す 502 は HTML。そこで `try` にすると
「502 だった」という情報が「JSON をデコードできない」に化ける。

## 204 のために `request` と `sendRequest` を分けた

```swift
func deleteMeal(id: UUID) async throws {
    try await requestNoContent("DELETE", "v1/meals/\(id.uuidString.lowercased())")
}

/// 本文を返さない経路（204）。読もうとすると decoding で落ちる
private func requestNoContent(_ method: String, _ path: String) async throws {
    _ = try await sendRequest(method, path, query: [], body: Optional<Never>.none)
}
```

`DELETE` は 204 No Content を返す。本文が空なので `JSONDecoder().decode()` が
必ず失敗する。**削除は成功しているのに `APIError.decoding` が飛ぶ**。

`request`（デコードする）と `sendRequest`（生の `Data` を返す）に割った。

`Optional<Never>.none` は「本文が無いことが型で確定している」書き方。
`Never` は `Encodable` に適合するので通る。

## エラー型

```swift
enum APIError: Error, LocalizedError {
    case http(status: Int, problem: Problem?)
    case transport(Error)
    case decoding(Error)
}
```

3つに割ってあるのは、**呼ぶ側の対処が違う**から。

| | 何が起きたか | 対処 |
|---|---|---|
| `transport` | 届かなかった | 積んで後で再送できる |
| `http` | 届いてサーバが断った | 内容による |
| `decoding` | 届いたが読めない | こちらのバグ。再送しても同じ |

### `isAlreadyRecorded` —— 再送を止める判定

```swift
var isAlreadyRecorded: Bool {
    guard case let .http(status, problem) = self else { return false }
    guard status == 409 || status == 422 else { return false }

    let text = (problem?.detail ?? "") + (problem?.title ?? "")

    return text.contains("既にある") || text.contains("既に存在する")
}
```

**「送信は届いたが応答が失われた」を検出する。** オフラインキューが
再送したとき、サーバは同じ id をすでに持っているので 409 を返す。
これを失敗として扱うと永久に送り続ける（[offline.md](offline.md)）。

**日本語のメッセージで判定しているのは弱い。** サーバ側の文言を変えると
壊れる。本来は `problem.type` に機械可読な URI を入れて判定すべきで、
そちらは `openapi.yaml` 側の課題として残っている。

## `HTTPTransport` を挟む理由

```swift
protocol HTTPTransport: Sendable {
    func send(_ request: URLRequest) async throws -> (Data, HTTPURLResponse)
}
```

`URLSession` を直接使うと、テストが**ネットワークと本番 DB の状態に依存する**。
`URLProtocol` を差し替える手もあるが、グローバルな設定をいじることになる。

1メソッドの protocol にしておけば、テストは

```swift
final class FakeTransport: HTTPTransport, @unchecked Sendable {
    private(set) var requests: [URLRequest] = []
    var responses: [(Data, Int)] = []
}
```

で済み、**送った `URLRequest` を検証できる**（URL・ヘッダ・本文）。
`AuthClient` も同じ `HTTPTransport` に載っているので、
Supabase SDK を入れずに済んでいる。
