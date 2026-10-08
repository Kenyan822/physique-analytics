# ADR-0018: 摂取目標を適用開始日つきの履歴で持つ

- Status: Accepted
- Date: 2026-10-08
- Issue: [#241](https://github.com/Kenyan822/physique-analytics/issues/241)

## Context

`manual_targets`（要件 N-05）は単一行（`id boolean primary key check(id)`）だった。目標を変えると前の値が消える。

食事記録は日付ごとに残るのに目標が1つしかないため、**過去日の残量が現在の目標で計算される**。

```
10/1  P150 を目標にして記録 → 達成していた
10/8  P180 に切り替え
      → 10/1 を開き直すと「P180 に対して150」= 未達 と表示される
```

3年分を後から振り返るのが目的なので、ここが狂うと分析にならない。

## Decision

**`manual_targets` を適用開始日つきの履歴テーブルにする。**

- `starts_on date not null unique`。その日の目標は `where starts_on <= $1 order by starts_on desc limit 1`
- 未来の日付で入れれば予約になる
- 同じ日に2回変えたら上書き（`starts_on` が unique）
- 履歴が0件の日、最初の `starts_on` より前の日は、今までどおり自動計算にフォールバックする

### `active` フラグを持たない

「active な行」と「`starts_on` が最新の行」は同値。別に持つと、active が2行ある・active なのに開始日が未来、といった**矛盾した状態を作れてしまう**。導出できるものは持たない。

### `mode = manual | auto` 列を持たない

「この日から自動計算に戻す」を表す列は、当初案にあったが外した。本番は `plan_phases` が0件で、自動計算（`weekly.Build`）は一度も値を出していない。使われていない経路のための列は今必要ない。必要になったら `alter table add column`（既存行は `default 'manual'`）で足せるので、**後回しのコストが低い**。

### `name` は nullable

`PUT /v1/targets/manual`（今日から）は名前を持たない。`not null` だと入れられない。

### 既存の1行の移行

`starts_on` は記録の最古日にする。`profile.start_date` は null で使えない。

```sql
least(min(meals.date), min(body_measurements.date), min(workout_sessions.date), current_date)
```

既存の全日が今までどおり同じ目標で評価され、**移行による表示の変化がゼロ**になる。

### API は既存3本の形を変えない

iOS（`APIClient.swift`）と Web が `GET/PUT/DELETE /v1/targets/manual` を直接使っている。形を変えると iOS が壊れる。issue 起票時は「`/v1/targets/{date}` 経由なので影響なし」としていたが誤りだった。

| | 意味 |
|---|---|
| `GET /v1/targets/manual` | `{ targets }`。**今日（JST）に有効な1件**または null |
| `PUT /v1/targets/manual` | **今日からこの目標にする**。`starts_on = 今日` を upsert |
| `DELETE /v1/targets/manual` | 手動目標をやめる。**履歴ごと全部消す** |
| `GET /v1/targets/manual/entries` | 履歴一覧（開始日の新しい順） |
| `POST /v1/targets/manual/entries` | `startsOn` 指定で1件追加（上書き）。予約もここ |
| `DELETE /v1/targets/manual/entries/{entryId}` | 履歴の1件を消す |

「今有効な目標」というビューと、その実体の集合を分けた形。過去日の目標は `GET /v1/targets/{date}` が内部でその日の履歴から引く。`GET /v1/targets/manual` に `?date=` は足さない。

## Consequences

### 良くなること

- 過去日の残量が当時の目標で出る
- 目標を変えても前の値が残る。後から振り返れる
- iOS・Web は無改修で動く

### 悪くなること・引き受けるリスク

**[ADR-0016](0016-auto-migrate-on-deploy.md) の「マイグレーションは追加のみ」に反する。** 000019 は旧テーブルをリネームして作り直し、旧を drop する。`migrate` → `deploy` の間に**旧コードが新スキーマを読む窓**ができ、その間 `GET/PUT /v1/targets/manual` と `/v1/targets/{date}` の手動目標部分が 500 になる。

expand/contract に分ければ避けられるが、次の理由で一括にした。

- 利用者は1人で、窓は `deploy-api` の数分
- 分けると「旧コードが読む `id boolean` 列を残したまま新列を足す」中間状態が要り、`active` を持たない設計と矛盾する状態を一時的に作ることになる

窓の間に失敗しても、`deploy` が終われば自然に復旧する（データは失われない）。

**`down` は履歴を落とす。** 今日時点で有効な1件だけが単一行に戻る。過去の履歴と予約は消える。ADR-0016 のとおり `down` は自動で流さない。

**`DELETE /v1/targets/manual` は履歴ごと消える。** 破壊的だが、「手動目標をやめる」と明示したときだけ。1件だけ消すときは `entries/{id}`。

**`updatedAt` は作成日時を返す。** 列は `created_at` だけ。同じ日の上書きでは変わらない。クライアントは使っていない。

## 却下した案

- **`active` フラグ＋複数行**: 上記のとおり矛盾した状態を作れる
- **日ごとに目標を持つ**: フェーズ単位で変えるもので過剰（元の N-05 と同じ判断）
- **`GET /v1/targets/manual` を履歴一覧に変える**: iOS が壊れる
