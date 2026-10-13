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
MODEL: gemini-3.8-flash
IMAGE_BYTES: 18
HAS_NOTE: True
API_KEY_SENT: True
```

`httptest.Server` でも同じことはできるが、**プロセスをまたいだ確認**には
環境変数で差し替えられる方が要る。ユニットテストは `http.Client` の
`Transport` を差し替えれば足りるので、両方を用意している。

## 提供者を替えるときに変わるのは「形」であって「構造」ではない

写真からの PFC 推定を Anthropic から Gemini に移した（[#301](https://github.com/Kenyan822/physique-analytics/issues/301)）。
`Config` / `Request` / `Estimate` と `Estimate()` / `Enabled()` はそのままで、
ハンドラ側は1文字も変えていない。**変わったのは送る JSON と読む JSON だけ。**

| | Anthropic | Gemini |
|---|---|---|
| モデルの指定 | ボディの `model` | **URL のパス** `/models/<model>:generateContent` |
| 鍵 | `x-api-key` | `x-goog-api-key` |
| 画像 | `content[].source.{type,media_type,data}` | `parts[].inline_data.{mime_type,data}` |
| 本文の取り出し | `content[].text` | `candidates[].content.parts[].text` |
| 出力の形を縛る | プロンプトで指示 | `generationConfig.responseFormat.text.schema` |

**モデル名が URL に入る**ので、宛先の組み立てが文字列連結になる。

```go
return fmt.Sprintf("%s/models/%s:generateContent",
	strings.TrimSuffix(base, "/"), url.PathEscape(c.cfg.Model))
```

`url.PathEscape` は `:` をエスケープしない（パスセグメント内では許される文字なので）。
`:generateContent` がそのまま残るのはこのため。モデル名だけを包んでおけば、
変な名前が来ても壊れない。

### 鍵は URL ではなくヘッダで渡す

Gemini はクエリパラメータ `?key=` でも通るが使わない。
**URL はプロキシとアクセスログに残る。** ヘッダなら残らない。

```go
httpReq.Header.Set("x-goog-api-key", c.cfg.APIKey)
```

テストでこれを見ておく。「ヘッダに入れた」だけでなく
**「URL に入っていない」も確かめる**（両方書ける状態だから）。

```go
if u := tr.gotReq.URL.String(); strings.Contains(u, "secret-key") {
	t.Errorf("URL に鍵が入っている: %s", u)
}
```

### `maxOutputTokens` は thinking の分も数える

Anthropic 版では「事故ったときのコストが上がる」という理由で `max_tokens: 512`
を付けていた。Gemini にそのまま移すと**壊れる。**

Gemini 3 系は思考を切れない（`gemini-3.8-flash` の既定は medium、
`minimal` を指定すると 400）。`maxOutputTokens` は思考トークンも含む
hard cutoff なので、512 だと**思考の途中で打ち切られ、本文が空のまま 200 が返る。**

```go
// 絞るなら maxOutputTokens ではなく thinkingLevel を下げる
"thinkingConfig": map[string]any{"thinkingLevel": "low"},
```

打ち切られたことは `candidates[].finishReason` に出る。
**200 なのに結果が無いときは理由をエラーに載せる。**

```go
return Estimate{}, fmt.Errorf("推定結果を読み取れなかった%s。もう一度撮るか、手で入力する",
	res.finishNote())   // （MAX_TOKENS）
```

### 無名構造体にメソッドは生やせない

応答の読み取りを最初はこう書いていた。

```go
func parseEstimate(raw []byte) (Estimate, error) {
	var res struct {
		Candidates []struct{ ... } `json:"candidates"`
	}
```

ここから `finishReason` を取り出す処理を切り出そうとして詰まった。
**無名構造体はメソッドのレシーバにできないし、関数の引数に書くと
同じ定義を丸ごと再掲することになる。**

使い捨ての形なら無名でよいが、**2か所から触るなら名前を付ける。**

```go
type generateContentResponse struct { ... }

func (r generateContentResponse) finishNote() string { ... }
```

### 対応する画像形式は提供者に合わせる

Anthropic は jpeg / png / webp / gif、Gemini は jpeg / png / webp / **heic** / heif。
**gif が落ちて heic が入る。** iPhone の写真はそのまま HEIC なので、変換が要らなくなった。

ここで1つ事故った。移行前のテストは「対応していない形式」の例に `image/heic` を
使っていて、**その MIME が許可リストに無いことに依存していた**。Gemini 版では
heic が通るようになり、テストが実際に API を叩きに行った（401 が返ってきて気づいた）。

```
err = 推定 API が 401 を返した: {"type":"error","error":{"type":"authentication_error",...
```

**「弾かれるはず」のテストは、弾かれなかったときに外へ出て行く。**
`http.DefaultClient` を渡しているとそのまま本番の URL に繋がる。
入口で止まることを確かめるテストでも、`Transport` は差し替えておく方が安全。

## 「使えない」は 503 で、理由と代替手段を返す

```go
return problem(503, "写真からの推定が未設定",
	"VISION_API_KEY と VISION_MODEL を設定すると使える。手で入力することもできる")
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

さらに **S3 互換の実装（MinIO）に実際に投げて確かめた。**

```
アップロード: 200
取得: 200 / 中身が一致: true
署名を壊した場合: 400（正しく検証されている）
```

これで「計算が合っている」だけでなく「サーバに受理される」ことまで
確かめられた。R2 も MinIO も SigV4 を話すので、実装の正しさとしては十分。

この過程で、`Endpoint` を指定しているのに `AccountID` が無いと
無効扱いになるバグが見つかった。**投げてみないと気づけない種類のもの。**

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
