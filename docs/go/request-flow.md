# リクエストが入ってから返るまで

`GET /v1/exercises?muscleGroup=胸` を例に、**TCP で受けてから JSON を書くまで**を
1本の線で追う。

構成そのものは [directory-layout.md](directory-layout.md)、
個別のトピックは [http-server.md](http-server.md) / [database.md](database.md)。
ここは**層と層の間**を扱う。

## 全体

```
net/http.Server
  └─ auth.Middleware            ← 自前。JWT を検証して context に sub を入れる
      └─ http.ServeMux          ← 生成物が HandleFunc で登録した
          └─ ServerInterfaceWrapper.ListExercises   ← 生成物。クエリを構造体に詰める
              └─ strictHandler.ListExercises        ← 生成物。型付きの request を作る
                  └─ handler.(*Server).ListExercises   ← 自前。ここが「実装」
                      └─ repository.(*Exercise).List   ← 自前。SQL
                          └─ pgxpool → Supavisor → Postgres
                      ◀── []openapi.Exercise
                  ◀── openapi.ListExercises200JSONResponse
              ◀── VisitListExercisesResponse が JSON を書く
```

**自前のコードは3か所だけ**（ミドルウェア・ハンドラ・リポジトリ）。
あいだは全部 `openapi.yaml` からの生成物（[codegen.md](codegen.md)）。

## 0. 配線（`cmd/server/main.go`）

```go
h := handler.NewRouter(handler.New(
    pool,
    repository.NewExercise(pool.DB()),
    repository.NewWorkout(pool.DB()),
    ...
))

if cfg.AuthDisabled {
    slog.Warn("認証が無効になっている。ローカル開発以外では使わない")
} else {
    v := auth.NewVerifier(cfg.SupabaseJWKSURL, cfg.AllowedUserIDs...)
    h = auth.Middleware(v, "/health")(h)
}

srv := &http.Server{
    Addr:              net.JoinHostPort("", strconv.Itoa(cfg.Port)),
    Handler:           h,
    ReadHeaderTimeout: 10 * time.Second,
}
```

**`main` は `new` と代入しかしていない。** リポジトリを13個渡しているのは
冗長に見えるが、`handler.Server` が自分で `pool` からリポジトリを作ると
テストで差し替えられなくなる。

`h` を再代入しているのは、**ミドルウェアが `http.Handler` を包む形**だから。
`auth.Middleware(v, "/health")` は `func(http.Handler) http.Handler` を返す。

## 1. `http.Server` が受ける

```go
ReadHeaderTimeout: 10 * time.Second,
```

**ヘッダを送り切らない接続にワーカーを占有させない。** これが無いと
Slowloris で簡単に詰まる。`ReadTimeout` を付けないのは、
CSV インポートのような大きい本文を切りたくないため。

## 2. 認証ミドルウェア

```go
func Middleware(v *Verifier, publicPaths ...string) func(http.Handler) http.Handler {
    public := make(map[string]bool, len(publicPaths))
    for _, p := range publicPaths {
        public[p] = true
    }

    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            if public[r.URL.Path] {
                next.ServeHTTP(w, r)
                return
            }

            token, ok := bearerToken(r.Header.Get("Authorization"))
            if !ok {
                unauthorized(w, "Authorization ヘッダが無いか Bearer ではない")
                return
            }

            claims, err := v.Verify(r.Context(), token)
            if err != nil {
                unauthorized(w, "トークンが受け入れられない")
                return
            }

            next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
        })
    }
}
```

### 2重クロージャの形

```
Middleware(v, "/health")   →  func(http.Handler) http.Handler
                    (h)    →  http.Handler
```

外側で**設定を閉じ込め**（`public` マップを1回だけ作る）、
内側が毎リクエスト走る。`public` の構築がリクエストごとに走らない。

### `/health` を素通しする理由

`openapi.yaml` で `security: []` にしてあるが、**生成物は認証を強制しない**
（`embedded-spec` を切っているので実行時のスペック検証が無い）。
ミドルウェア側でパスを持つしかない。

keepalive（[docs/05](../05-インフラ設計.md)）と Cloud Run のスモークテストが叩く。

### 検証は2段

```go
func (v *Verifier) Verify(ctx context.Context, token string) (Claims, error) {
    claims, err := v.parse(ctx, token)      // ① 署名・期限
    if err != nil {
        return Claims{}, fmt.Errorf("%w: %w", ErrUnauthorized, err)
    }
    if !v.allowed[claims.UserID] {          // ② 許可リスト
        return Claims{}, fmt.Errorf("%w: 許可されていない利用者", ErrUnauthorized)
    }

    return claims, nil
}
```

**①だけでは足りない。** 署名の検証が言えるのは「Supabase が発行した」まで。
Supabase のサインアップが開いていれば、登録した誰もが有効なトークンを持てる（#51）。

②で `sub` を絞る。**空なら誰も通さない**（#152）。
「設定するまで開いている」は「ログインするまで開いている」と同じ。

### JWKS のキャッシュ

```go
func (v *Verifier) keyFor(ctx context.Context, kid string) (*ecdsa.PublicKey, error) {
    v.mu.RLock()
    key, ok := v.keys[kid]
    fresh := time.Since(v.fetchedAt) < jwksTTL
    v.mu.RUnlock()

    if ok && fresh { return key, nil }

    if err := v.refresh(ctx); err != nil {
        // 取り直しに失敗しても、手元に鍵があれば使う。
        // JWKS の一時的な障害で全リクエストが落ちるのを避ける
        if ok { return key, nil }
        return nil, err
    }
    ...
}
```

`RWMutex` で読みは並行。TTL は1時間。**取得に失敗しても古い鍵を使う**のは、
Supabase 側の一時障害でサービス全体が落ちるのを避けるため。

### `context` に `sub` を入れる

```go
next.ServeHTTP(w, r.WithContext(WithUserID(r.Context(), claims.UserID)))
```

`r.WithContext` は**新しい `*http.Request` を返す**（元は変わらない）。
ハンドラは `auth.UserID(ctx)` で取れる。

いまは使っていない（テーブルに `user_id` 列が無い、#51）。
入れてあるのは、行ごとの分離を入れるときの受け皿。

## 3. ルーティング（生成物）

```go
m.HandleFunc(http.MethodGet+" "+options.BaseURL+"/v1/exercises", wrapper.ListExercises)
m.HandleFunc(http.MethodGet+" "+options.BaseURL+"/v1/exercises/{exerciseId}", wrapper.GetExercise)
```

**Go 1.22 の `ServeMux` はメソッドとパス変数を書ける。**
`"GET /v1/exercises/{exerciseId}"` がそのままパターンになる。
chi も gorilla も要らない（[http-server.md](http-server.md)）。

より具体的なパターンが優先されるので、登録順を気にしなくてよい。

## 4. パラメータの束縛（生成物）

```go
func (siw *ServerInterfaceWrapper) ListExercises(w http.ResponseWriter, r *http.Request) {
    var params ListExercisesParams

    err = runtime.BindQueryParameterWithOptions("form", true, false, "muscleGroup",
        r.URL.Query(), &params.MuscleGroup, ...)
    if err != nil {
        var requiredError *runtime.RequiredParameterError
        if errors.As(err, &requiredError) {
            siw.ErrorHandlerFunc(w, r, &RequiredParamError{ParamName: "muscleGroup"})
        } else {
            siw.ErrorHandlerFunc(w, r, &InvalidParamFormatError{ParamName: "muscleGroup", Err: err})
        }
        return
    }
    ...
}
```

`?muscleGroup=胸` が `*openapi.MuscleGroup` に入る。**手で `r.URL.Query().Get()` を
書かない。** 型が `openapi.yaml` の enum と一致していることは生成が保証する。

失敗したときの出口は `main` 側で決めている。

```go
return openapi.HandlerWithOptions(strict, openapi.StdHTTPServerOptions{
    BaseRouter: http.NewServeMux(),
    ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
        writeProblem(w, http.StatusBadRequest, "リクエストが不正", err.Error())
    },
})
```

**ここは 400 でよい。** 原因は送った側にあるので、内容を返しても情報漏洩にならない。

## 5. 型付きリクエストに詰め替える（生成物）

```go
func (sh *strictHandler) ListExercises(w http.ResponseWriter, r *http.Request, params ListExercisesParams) {
    var request ListExercisesRequestObject
    request.Params = params

    handler := func(ctx context.Context, w http.ResponseWriter, r *http.Request, request interface{}) (interface{}, error) {
        return sh.ssi.ListExercises(ctx, request.(ListExercisesRequestObject))
    }
    for _, middleware := range sh.middlewares {
        handler = middleware(handler, "ListExercises")
    }

    response, err := handler(r.Context(), w, r, request)

    if err != nil {
        sh.options.ResponseErrorHandlerFunc(w, r, err)
    } else if validResponse, ok := response.(ListExercisesResponseObject); ok {
        if err := validResponse.VisitListExercisesResponse(w); err != nil {
            sh.options.ResponseErrorHandlerFunc(w, r, err)
        }
    }
}
```

**`w` と `r` がまだ渡ってきているが、実装側は受け取らない。**
`StrictServerInterface` の署名は `(ctx, request) (response, error)` だけ。

これが strict server の効き目で、ハンドラは
**`http.ResponseWriter` に触れない ＝ 仕様外のレスポンスを書けない**。

## 6. ハンドラ（自前）

```go
func (s *Server) ListExercises(ctx context.Context, req openapi.ListExercisesRequestObject) (openapi.ListExercisesResponseObject, error) {
    f := repository.ExerciseFilter{MuscleGroup: req.Params.MuscleGroup}
    if req.Params.IncludeDeleted != nil {
        f.IncludeDeleted = *req.Params.IncludeDeleted
    }

    items, err := s.exercises.List(ctx, f)
    if err != nil {
        return nil, err
    }
    if items == nil {
        items = []openapi.Exercise{}
    }

    return openapi.ListExercises200JSONResponse{Items: items}, nil
}
```

やっているのは3つだけ。

1. **API の型 → リポジトリの型**（`ListExercisesParams` → `ExerciseFilter`）
2. 呼ぶ
3. **API の型に戻す**

### `items == nil` を潰す

**Go の `nil` スライスは `null` にエンコードされる。** `[]` ではない。

```go
var s []int
json.Marshal(s)        // null
json.Marshal([]int{})  // []
```

クライアントが `items.map(...)` する前提なので、空配列にして返す。
`openapi.yaml` でも `items` は必須。

### エラーは返すだけ

`return nil, err` すると、生成物が `ResponseErrorHandlerFunc` を呼ぶ。

```go
func handleResponseError(w http.ResponseWriter, r *http.Request, err error) {
    // 内部エラーの中身はログにだけ残す。接続先やクエリが応答に混ざるのを避ける
    slog.ErrorContext(r.Context(), "ハンドラがエラーを返した",
        slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
    writeProblem(w, http.StatusInternalServerError, "サーバ内部エラー", "")
}
```

**`detail` を空にしている。** `fmt.Errorf("種目の一覧を引けない: %w", err)` の
中身には DSN やクエリが混ざりうる。ログにだけ残す。

### 「見つからない」はエラーではなく応答

```go
e, err := s.exercises.Get(ctx, req.ExerciseId)
if repository.IsNotFound(err) {
    return openapi.GetExercise404ApplicationProblemPlusJSONResponse{...}, nil
}
```

**`error` で返すと 500 になる。** 404 は正常な応答なので、
レスポンス型として返して `err` は `nil`。

`repository.IsNotFound` が挟まっているのは、`pgx.ErrNoRows` を
ハンドラに漏らさないため（[database.md](database.md#pgxerrnorows-を上の層に漏らさない)）。

## 7. リポジトリ（自前）

```go
const q = `
    select ` + exerciseColumns + `
    from exercises
    where ($1::muscle_group is null or muscle_group = $1::muscle_group)
      and ($2::boolean or deleted_at is null)
    order by name`

rows, err := r.db.Query(ctx, q, mg, f.IncludeDeleted)
if err != nil {
    return nil, fmt.Errorf("種目の一覧を引けない: %w", err)
}
defer rows.Close()
```

**条件分岐を SQL に寄せている。** Go 側で `WHERE` を組み立てると
条件が増えるたびに文字列連結の分岐が増え、プレースホルダの番号がずれる。

`$1::muscle_group is null or ...` なら**クエリは常に1つ**で、
プリペアドステートメントのキャッシュも効く。

### `r.db` の型は `DBTX`

```go
type Exercise struct {
    db DBTX
}
```

`*pgxpool.Pool` でも `pgx.Tx` でも入る。テストがトランザクションを渡して
**ロールバックで隔離**できる（[database.md](database.md)）。

## 8. JSON を書く（生成物）

```go
func (response ListExercises200JSONResponse) VisitListExercisesResponse(w http.ResponseWriter) error {
    var buf bytes.Buffer
    if err := json.NewEncoder(&buf).Encode(response); err != nil {
        return err
    }
    w.Header().Set("Content-Type", "application/json")
    w.WriteHeader(200)
    _, err := buf.WriteTo(w)

    return err
}
```

**いったん `bytes.Buffer` に書いてから流す。** 直接 `w` にエンコードすると、
途中で失敗したときに**ステータス 200 と壊れた JSON が出てしまう**
（`WriteHeader` は一度しか効かない）。

## 終了するとき

```go
select {
case err := <-errCh:
    return err
case <-ctx.Done():
    slog.Info("終了シグナルを受け取った。処理中のリクエストを待つ")
}

// Cloud Run は SIGTERM の 10 秒後に強制終了する。それより短く切り上げる
shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
defer cancel()
return srv.Shutdown(shutdownCtx)
```

`signal.NotifyContext` で SIGTERM を `ctx` に変換している。
`srv.Shutdown` は**新規接続を止め、処理中のリクエストを待つ**。

`context.Background()` から作り直しているのが要点。
`ctx` は既にキャンセル済みなので、そのまま渡すと即座に打ち切られる。
