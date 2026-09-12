# net/http でサーバを書く

## ルーティングは標準の `ServeMux` で足りる

Go 1.22 で `ServeMux` がメソッドとパスパラメータに対応した。

```go
mux := http.NewServeMux()
mux.HandleFunc("GET /v1/exercises/{exerciseId}", h)
// r.PathValue("exerciseId") で取れる
```

chi や gin を入れる理由が「パスパラメータが欲しい」だけなら、もう標準で足りる
（[docs/06-技術選定.md](../06-技術選定.md)）。

## `http.Server` は必ずタイムアウトを設定する

```go
srv := &http.Server{
	Addr:              ":8080",
	Handler:           h,
	ReadHeaderTimeout: 10 * time.Second,
}
```

`http.ListenAndServe` の既定はタイムアウトなし。
ヘッダを送り切らない接続を張られるとゴルーチンが解放されない（Slowloris）。
`gosec` の G112 がこれを検出する。

## グレースフルシャットダウン

Cloud Run はインスタンスを止めるとき SIGTERM を送り、**10秒後に強制終了**する。
処理中のリクエストを取りこぼさないために、シグナルを受けたら新規受付を止めて待つ。

```go
ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
defer stop()

go func() {
	if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		errCh <- err
	}
}()

<-ctx.Done()

// 強制終了より短く切り上げる
shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
defer cancel()
return srv.Shutdown(shutdownCtx)
```

- `signal.NotifyContext` — シグナルで cancel される context を作る。`signal.Notify` + チャネルより短い
- `ListenAndServe` は `Shutdown` されると `http.ErrServerClosed` を返す。**これは正常終了**なので
  `errors.Is` で弾く。弾かないと終了のたびにエラーログが出る
- `Shutdown` に渡す context は `ctx` ではなく新しく作る。`ctx` はもう cancel 済みで、
  渡すと即座に打ち切られてシャットダウンを待てない

## `main` は薄くして `run() error` に寄せる

```go
func main() {
	if err := run(); err != nil {
		slog.Error("起動に失敗", slog.Any("error", err))
		os.Exit(1)
	}
}
```

`os.Exit` は `defer` を実行しない。`main` の中で直接 `os.Exit` すると
`defer pool.Close()` が飛ぶ。エラーを返す `run()` に処理を寄せて、
`os.Exit` は `main` の一箇所だけにする。

## 構造化ログは `log/slog`（標準）

```go
slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))
slog.ErrorContext(ctx, "DB への疎通に失敗", slog.Any("error", err))
```

Go 1.21 から標準。zap / zerolog を入れなくてよい。
Cloud Logging は stdout の JSON を構造化ログとして取り込むので、JSONHandler をそのまま使う。
