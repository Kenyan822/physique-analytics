# 08. MCP — Claude Code から分析する

**このプロジェクトの分析UIは MCP サーバーである。**画面は作らない（[ADR-0010](adr/0010-mcp-over-analysis-ui.md)）。

## 1. なぜ画面ではないのか

知りたいことが**事前に定義できない**から。

> 睡眠6時間未満だった翌日の e1RM は平均どれくらい落ちるか
> 炭水化物を増やした週はトン数が伸びているか
> 停滞している種目は、ボリュームと睡眠のどちらと相関しているか

これらは思いついた時点で初めて形になる質問で、固定の画面では構造的に答えられない。
グラフを10枚足しても11枚目が欲しくなる。

**代わりに DB への問い合わせ口を Claude Code に渡す。** Web に置くのは毎日見る少数の
指標だけに留める（[ADR-0009](adr/0009-web-first-with-input.md)）。

```
あなた ──「先月と今月で胸のボリューム変わった？」──▶ Claude Code
                                                      │
                                                      ├─ weekly_volume（固定ツール）
                                                      └─ query（任意の SQL）
                                                            │
                                                            ▼
                                                      Postgres（手元 or Supabase）
```

## 2. なぜ API ではなく DB を直接読むのか

MCP サーバーは**手元でしか動かさない**。API を経由すると、分析のたびに
「HTTP のページング」「認証トークンの寿命」「エンドポイントの追加」が挟まる。

DB を直接読めば、`query` ツール1つで任意の集計ができる。
分析ロジック（e1RM・MEV/MRV 判定など）は `internal/analytics` の純粋関数を
API と共有しているので、**出る数字は API と同じ**（[ADR-0011](adr/0011-go-analytics.md)）。

## 3. 登録する

```bash
cd api && go build -o ~/.local/bin/physique-mcp ./cmd/mcp

claude mcp add physique -s user \
  -e DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' \
  -- ~/.local/bin/physique-mcp
```

登録後は**セッションを再起動**する（MCP サーバーは起動時にしか接続されない）。
`/mcp` に `physique` が出れば成功。

### 注意

**`/tmp` にビルドしない。** 再起動で消えて、次のセッションで繋がらなくなる。

**本番（Supabase）を見たいときは `DATABASE_URL` を差し替える。**
その場合は Session pooler ではなく Transaction pooler（6543）でよい。
読むだけなので advisory lock は要らない。

**個人設定（`private/config.json`）は任意。** 無くても起動する。
あると週次レポートがフェーズや大会を反映する（§4 の `weekly_report` / `weekly_actions`）。

## 4. ツール

8個。**固定ツールは「よく聞くこと」への近道**で、そこから外れたら `query` を使う。

### 記録を引く

| ツール | 内容 | 聞き方の例 |
|---|---|---|
| `list_exercises` | 種目マスタ（58種目・13部位） | 「胸の種目を一覧して」 |
| `list_workout_sessions` | 期間指定でトレーニング記録。セットを含む | 「先週のトレーニングを見せて」 |

`list_exercises` が返す**種目 ID は他のツールの引数になる**。

### 分析する

| ツール | 内容 | 聞き方の例 |
|---|---|---|
| `weekly_volume` | 部位別の週間セット数・トン数と MEV/MRV 判定 | 「今週のボリュームは適正？」 |
| `exercise_progress` | 種目の推定1RM（Epley + RIR補正）の推移と傾き | 「ベンチの1RMは伸びてる？」 |
| `correlations` | 個人の反応を見る相関分析（要件 A-12） | 「睡眠とトン数って関係ある？」 |

**`weekly_volume` の判定は「振り替え」を前提にしている。**
MRV を超えた部位を削って MEV 以下の部位に回すのが正しい読み方で、
総量を増やす方向には使わない。

**`exercise_progress` は一部のセットを捨てている。** RIR 未記録と、限界12レップ超は
Epley の推定が崩れるため除外している。傾きが出ないときは「RIR を記録すると増える」と返る。

### まとめて読む

| ツール | 内容 | 聞き方の例 |
|---|---|---|
| `weekly_report` | 週次レポートを Markdown で（要件 A-13） | 「今週のレポートを出して」 |
| `weekly_actions` | 今週のアクション（要件 A-08） | 「来週何を変えればいい？」 |

**`weekly_actions` が最終成果物**（[docs/01-要件定義.md](01-要件定義.md)）。
数字の羅列ではなく「来週何を変えるか」が優先順位つきで出る。
体重トレンドから TDEE を逆算し、停滞検知・大会カウントダウン・回復判定を統合する。

### 何でも聞く

| ツール | 内容 |
|---|---|
| `query` | **読み取り専用の SQL**。事前に定義できない分析はこれ |

固定ツールで答えられない質問は、Claude が自分で SQL を書いて投げる。

```sql
-- 「睡眠6時間未満の翌日はトン数が落ちるか」の例
select
  case when d.sleep_h < 6 then '6h未満' else '6h以上' end as 睡眠,
  round(avg(t.tonnage)) as 平均トン数,
  count(*) as 日数
from daily_metrics d
join (
  select s.date, sum(ws.weight_kg * ws.reps) as tonnage
  from workout_sessions s
  join workout_sets ws on ws.session_id = s.id and ws.deleted_at is null
  where s.deleted_at is null
  group by s.date
) t on t.date = d.date + 1
where d.deleted_at is null and d.sleep_h is not null
group by 1
```

## 5. `query` の安全策

読み取り専用を**3段**で守っている。

| | 何をするか | なぜ要るか |
|---|---|---|
| 1 | `select` / `with` で始まることを要求 | 明らかな書き込みを弾く |
| 2 | 書き込み系キーワードを含むものを弾く | **1 だけだと CTE に隠せる**: `with x as (delete from ... returning id) select * from x` |
| 3 | `;` を含むものを弾く | **`select 1; delete from exercises` が通ってしまう** |

単語境界（`\b`）で見ているので、`created_at` のような列名には反応しない。
返す行数にも上限がある。

### これで完全ではない

正しいのは**接続自体を読み取り専用ロールにする**こと。キーワードのブラックリストは、
関数経由の副作用や将来の構文追加に追随できない。

**手元でしか動かさない前提なので今はこれで足りている。**
MCP をリモートに置くなら読み取り専用ロールが必須になる。

## 6. テーブル

`query` で使えるもの。**すべて `deleted_at` による論理削除**なので、生きている行だけ
見るなら `deleted_at is null` を付ける（[ADR-0014](adr/0014-sync-conflict-resolution.md)）。

| 分類 | テーブル |
|---|---|
| トレーニング | `exercises` / `exercise_aliases` / `workout_sessions` / `workout_sets` / `templates` / `template_items` |
| 体組成 | `daily_metrics` / `body_measurements` / `body_photos` |
| 栄養 | `meals` / `meal_sets` / `meal_set_items` |
| 計画 | `profile` / `plan_phases` / `plan_blocks` / `nutrition_settings` / `volume_ranges` / `contests` |
| 検査 | `blood_tests` / `blood_test_items` |

日付は `workout_sessions.date` / `daily_metrics.date` など。**すべて JST の日付**
（[ADR-0013](adr/0013-timezone-jst.md)）。`timestamptz` と混同しない。

構造の意図は [docs/02-データモデル.md](02-データモデル.md)。

## 7. 動かないとき

| 症状 | 原因 |
|---|---|
| `/mcp` に出ない | セッションを再起動していない。MCP は起動時にしか接続されない |
| `ENOENT` | バイナリのパスが消えた。`/tmp` に置いていないか確認する |
| 起動はするが応答しない | `DATABASE_URL` が違う。`docker compose up -d` を忘れていないか |
| ツールは見えるが結果が空 | `deleted_at is null` を付けていないか、逆に日付の範囲が外れている |

**stdout に何か出力するとプロトコルが壊れる。** MCP は stdout で JSON-RPC を
やり取りするので、サーバー側のログは stderr に出している（[docs/go/mcp.md](go/mcp.md)）。
`fmt.Println` を足すと動かなくなる。

## 8. 実装

- 実装の記録: [docs/go/mcp.md](go/mcp.md)（ツール設計・`query` のガード・テストの書き方）
- コード: `api/internal/mcpserver/` / `api/cmd/mcp/`
- 決定: [ADR-0010](adr/0010-mcp-over-analysis-ui.md)（分析UIを作らない）
