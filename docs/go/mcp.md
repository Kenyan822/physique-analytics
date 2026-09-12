# MCP サーバ

Claude Code から分析するためのサーバ（[ADR-0010](../adr/0010-mcp-over-analysis-ui.md)）。
`api/cmd/mcp` と `api/internal/mcpserver`。

## なぜ API ではなく DB を直接読むのか

MCP は**手元でしか動かない**。stdio で Claude Code と話すプロセスなので、

- HTTP を待ち受けない → 認証を挟む対象が無い
- API 経由にすると Cloud Run 往復とレート制限が挟まる
- 「CSV に出して分析する」往復が無くなるのが MCP を前倒しした理由そのもの

なので `repository` を直接使う。`config.LoadForMCP` が `SUPABASE_JWKS_URL` を
要求しないのはこのため。

## ツールの設計

```go
mcp.AddTool(s, &mcp.Tool{
	Name:        "weekly_volume",
	Description: "部位別の週間セット数とトン数。MEV / MRV と突き合わせた判定を返す。" +
		"MRV超の部位は削って MEV以下の部位に振り替える。総量を増やしてはいけない。",
}, func(ctx context.Context, _ *mcp.CallToolRequest, in weeklyVolumeIn) (*mcp.CallToolResult, weeklyVolumeOut, error) {
	...
})
```

### Description に「どう解釈するか」まで書く

数値だけ返しても、Claude は「セット数が多いのは良いこと」と解釈しかねない。
**MRV超は減らすべき状態**であり、総量を増やす方向の提案は誤り。
そこまで Description に書く。

ツールの説明文は、**その値を読む人間に渡す注意書きと同じもの**を書けばよい。

### 入出力は専用の構造体にする

```go
type weeklyVolumeIn struct {
	From string `json:"from,omitempty" jsonschema:"開始日 YYYY-MM-DD（JST）。省略時は7日前"`
	To   string `json:"to,omitempty" jsonschema:"終了日 YYYY-MM-DD（JST）。省略時は今日"`
}
```

`jsonschema` タグが JSON Schema の description になり、そのまま Claude に渡る。
**`openapi.WorkoutSession` をそのまま返さない。** 生成型は `deletedAt` や
`createdAt` を含んでいて、分析には要らないのにコンテキストを食う。

### 既定値は「省略できる」形にする

`from` / `to` を必須にすると毎回日付を組み立てさせることになる。
省略時は「7日前〜今日」に落とす。**今日は JST で決める**（ADR-0013）。
UTC で `time.Now()` すると日本時間の朝9時前に1日ずれる。

## `query` ツール — 事前に定義できない質問への口

ADR-0010 の理由は「知りたいことが事前に定義できない」。
固定ツールだけではその前提を満たせないので、読み取り専用の SQL を通す口を置いた。

### 3段で守る

```go
// 1. select / with で始まることを要求
if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") { ... }

// 2. 書き込み系のキーワードを含むものを弾く
var writeKeywords = regexp.MustCompile(`(?i)\b(insert|update|delete|drop|truncate|alter|create|grant|...)\b`)

// 3. 複数文を弾く
if strings.Contains(body, ";") { ... }
```

**2 が要る理由**は、1 だけだと CTE に書き込みを隠せるから。

```sql
with x as (delete from exercises returning id) select * from x
--  ^^^^^^ select で始まっていないが with で始まっている
```

**3 が要る理由**は、`select 1; delete from exercises` が通ってしまうから。

単語境界（`\b`）で見ているのは、`created_at` のような列名に反応しないため。

### これでも完全ではない

正しいのは**接続自体を読み取り専用ロールにする**こと。
キーワードのブラックリストは、関数経由の書き込み（`select pg_sleep(...)` の類）や
将来の構文追加に追随できない。

手元でしか動かさない前提なので今はこれで足りるが、
MCP をリモートに置くなら読み取り専用ロールが必須になる。

## 確認の仕方

MCP は JSON-RPC over stdio なので、プロセスに直接流し込めば確かめられる。

```python
p = subprocess.Popen(["./mcp"], stdin=PIPE, stdout=PIPE, env={"DATABASE_URL": ...})
send({"jsonrpc":"2.0","id":1,"method":"initialize","params":{
    "protocolVersion":"2025-06-18","capabilities":{},
    "clientInfo":{"name":"probe","version":"0"}}})
send({"jsonrpc":"2.0","method":"notifications/initialized"})   # ← これを忘れると tools/list が返らない
send({"jsonrpc":"2.0","id":2,"method":"tools/list"})
```

**`notifications/initialized` を送るまでサーバは要求を受け付けない。**
初期化のハンドシェイクが完了していないため。ここで詰まりやすい。

## stdout を使わない

```go
// stdout は MCP のプロトコルが使う。ログは stderr へ出す
slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))
```

`fmt.Println` を1つ入れるだけで JSON-RPC のストリームが壊れる。
API サーバ（`cmd/server`）は stdout に JSON ログを出すので、逆になっている。

## 個人設定は任意の依存にする

```go
type Deps struct {
	Exercises *repository.Exercise
	Analysis  *repository.Analysis

	// nil でも他のツールは動く。weekly_actions だけが要求する
	Plan *plan.Plan
}
```

`weekly_actions`（要件 A-08）は目標ペースと PFC 係数が要るが、これらは
`private/config.json` にある個人データ（[ADR-0002](../adr/0002-separate-personal-data.md)）。
起動時に必須にすると、設定を持たない環境（CI・他人の手元）で
MCP 自体が立ち上がらなくなる。

**nil を許して、使うツールの中でエラーにする。**

```go
if d.Plan == nil {
	return weeklyActionsOut{}, fmt.Errorf(
		"計画の設定が読めていない。PHYSIQUE_CONFIG に config.json のパスを設定する")
}
```

エラーメッセージに**直し方**を書く。MCP のエラーは Claude が読んで
利用者に伝えるので、「設定が無い」だけでは次の行動が決まらない。

## 内部テストでツールの中身を検証する

```go
// 内部テストにしているのは、weeklyActions を MCP のセッションを張らずに呼ぶため
package mcpserver

func TestWeeklyActions_記録が無ければ記録を促す(t *testing.T) {
	d, _ := fixture(t, true)
	got, err := weeklyActions(t.Context(), d, weeklyActionsIn{AsOf: "2028-06-15"})
	...
}
```

`mcp.AddTool` に渡すクロージャを直接テストしようとすると、クライアント
セッションを張る必要が出る。**登録の薄い層と中身を分けて**、中身だけを
内部テスト（`package mcpserver`）から呼ぶ。

テストのためだけに関数を export しないで済むのが内部テストの利点。
`_test` パッケージを既定にするのは公開APIの使い勝手を確かめるためなので、
その目的が無いならこちらでよい。

## テストの日付は「実データと重ならない年」を選ぶ

手元の DB にはサンプルデータがコミットされている。`testdb.Begin` は
トランザクションで隔離するが、**コミット済みの行は見える**。

2026-09〜10 にサンプルがあるなら、テストは 2028 年で書く。
「記録が無いときの挙動」を確かめるテストが、手元でだけ落ちるのを防ぐ。
