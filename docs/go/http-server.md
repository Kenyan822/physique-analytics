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

## 外部 API のクライアントは宛先を差し替えられるようにする

```go
type Config struct {
	APIKey   string
	Model    string
	Endpoint string  // 空なら本番の既定
}
```

**課金が発生する経路を、課金なしで通せるようにするため。**
偽のサーバを立てて `VISION_ENDPOINT` を向ければ、リクエストの中身
（モデル名・画像のバイト数・補足テキストが入っているか・API キーが
付いているか）まで手元で確かめられる。

```
MODEL: claude-haiku-4-5-20251001
IMAGE_BYTES: 18
HAS_NOTE: True
API_KEY_SENT: True
```

`httptest.Server` でも同じことはできるが、**プロセスをまたいだ確認**には
環境変数で差し替えられる方が要る。ユニットテストは `http.Client` の
`Transport` を差し替えれば足りるので、両方を用意している。

## 「使えない」は 503 で、理由と代替手段を返す

```go
return problem(503, "写真からの推定が未設定",
	"ANTHROPIC_API_KEY と VISION_MODEL を設定すると使える。手で入力することもできる")
```

機能が無効なのはリクエストのせいではないので 4xx にしない。
**直し方と、今すぐ進む方法の両方を書く。** 「使えません」だけだと、
利用者はそこで止まる。

## 署名の実装は独立に検算する

SigV4 を手で実装したので、**Go とは別に Python で同じ署名を計算して
突き合わせた**。

```
Python が計算した署名: 3d4761a36938f0c2767e94c502e9102efdc8eff59ef7b2920eee67d49249ab7a
Go が計算した署名:     3d4761a36938f0c2767e94c502e9102efdc8eff59ef7b2920eee67d49249ab7a
```

暗号まわりは「動いているように見えて署名が違う」が起こり、
**実際のサービスに投げるまで気づけない**。自分のテストは自分の実装を
なぞるだけなので、別の実装と突き合わせる価値がある。

同じ理由で、分析ロジックは reference の Python と突き合わせている（ADR-0011）。

## 大きいファイルは API を通さない

```
POST /v1/photos → 署名付きURL を返す
                → クライアントが直接 PUT
```

4MB の画像を Cloud Run に通すと、メモリもリクエスト時間も無駄になる。
**メタデータだけ API が持ち、実体はクライアントとストレージが直接やり取りする。**

順序は「先にメタデータを作る → URL を返す」にした。逆にすると、
アップロードされたが DB に無いオブジェクトが残る。
