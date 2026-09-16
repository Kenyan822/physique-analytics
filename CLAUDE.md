# physique-analytics

3年計画の肉体改造を支えるデータ基盤。トレーニング・栄養・体組成を統合して分析する。
全体像は [README.md](README.md)、設計判断は [docs/adr/](docs/adr/) を参照。

## 絶対に守ること

- **`private/` の中身をコミットしない。** 個人の体組成・身体写真・血液検査結果が入っている（[ADR-0002](docs/adr/0002-separate-personal-data.md)）
- **個人の実数値を public 側に書かない。** 身長・体重・体脂肪率・挙上重量をコードやドキュメントにハードコードしない。設定は `private/config.json` から読み、公開用の例は `config.example.json` の値を使う
- 実データが必要な作業は `private/config.json` / `private/data/` を参照する。サンプルは `data/sample/`

## Go を書いたら学びを記録する

**このプロジェクトは Go の学習を兼ねている**（[ADR-0006](docs/adr/0006-go-backend.md)）。Go のコードを書いた・変更したときは、学んだことを `docs/go/` に記録する。

### 記録する対象

**学習目的なので、文法メモも含めて幅広く書いてよい。** 自分の言葉で書くこと自体が学習になり、検索し直すより手元にある方が速い。

- 文法・標準ライブラリの使い方（`defer` の評価タイミング、スライスの挙動など）
- イディオム（なぜ Go ではこう書くのか）
- Python / TypeScript と判断が異なった点
- ハマった点と、その原因

### どこに書くか

| | 置き場所 | 公開 |
|---|---|---|
| **Go の実装で学んだこと** | `docs/go/<トピック>.md` | **する**（ポートフォリオの一部） |
| **Swift / iOS の実装で学んだこと** | `docs/swift/<トピック>.md` | **する**（同上） |
| **サービス・概念の理解メモ**（Sentry とは何か、等） | `private/learning/<トピック>.md` | しない |

Swift も同じ扱いにする（[docs/swift/README.md](docs/swift/README.md)）。
索引は各 README の表に追加する。

### 書き方

`docs/go/<トピック>.md` に追記する。トピックが無ければ新規作成し、[docs/go/README.md](docs/go/README.md) の索引に追加する。

エントリは2種類。内容に応じて使い分ける。

**A. 文法・イディオムのメモ** — コード例と短い説明

```markdown
## スライスの append は元の配列を書き換えることがある

​```go
a := []int{1, 2, 3}
b := append(a[:1], 4)   // a も [1 4 3] に変わる
​```

容量に余裕があると同じ配列を再利用するため。切り出して渡すときは
`slices.Clone` するか、`a[:1:1]` で容量を切る。
```

**B. 判断の記録** — 設計や実装方針で迷った場合

```markdown
## <何をしたか / 何にハマったか>

**状況**: どのコードを書いていて、何が起きたか
**判断**: どう書いたか
**理由**: なぜそう書くのか。他言語との違いがあれば併記
```

## パッケージマネージャ

| 言語 | 使う | 使わない |
|---|---|---|
| TypeScript / Node.js | **pnpm** | **npm / yarn** |
| Go | `go mod`（標準） | — |
| Python（`reference/` のみ） | `pip` | — |

**`npm` / `yarn` のコマンドを実行しない。** 対応は以下。

| npm | pnpm |
|---|---|
| `npm install` | `pnpm install` |
| `npm install <pkg>` | `pnpm add <pkg>` |
| `npm run <script>` | `pnpm <script>` |
| `npx <cmd>` | **`pnpm dlx <cmd>`** |
| `npm ci` | `pnpm install --frozen-lockfile` |

### なぜ pnpm か

1. **宣言していない依存を import できない。** npm / yarn は hoisting により、`package.json` に書いていないパッケージも import できてしまう（phantom dependency）。pnpm はこれを構造的に防ぐ
2. **ディスク効率。** グローバルストアからハードリンクするため、プロジェクトが増えても容量が膨らまない
3. **monorepo の workspace サポート**が標準

### 強制の仕組み

書くだけでは守られないため、3重で防ぐ。

- `package.json` の **`packageManager`** フィールド（Corepack が読んで固定する）
- **`preinstall: "npx only-allow pnpm"`**（`npm install` を実行すると弾かれる）
- `.gitignore` で **`package-lock.json` / `yarn.lock` を除外**（誤コミットの防止）

## TDD で進める

**実装を書く前にテストを書く。** 順序は Red → Green → Refactor。

1. **Red**: 失敗するテストを書き、**実際に失敗することを確認する**
2. **Green**: テストが通る最小の実装をする
3. **Refactor**: テストが通ったまま整える

### 失敗を確認する理由

**テストが最初から通ってしまう場合、そのテストは何も検証していない。** 実装前に必ず実行して、意図した理由で失敗することを見る。「書いたつもりのテストが実は空振りしていた」を防ぐ唯一の方法。

### 適用範囲

| 対象 | TDD |
|---|---|
| **分析ロジック**（`api/internal/analytics`） | **必須** |
| **API ハンドラ** | **必須** |
| **DB アクセス層** | **必須** |
| UI（Web / iOS） | 任意 |

分析ロジックは仕様が [docs/03-分析ロジック.md](docs/03-分析ロジック.md) に明文化されているため、**仕様がそのままテストケースになる**。TDD が最も機能する領域。

UI のテストは費用対効果が低いため必須としない。

### Go でのテストの書き方

**テーブル駆動テスト**を基本とする。ケースの追加が1行で済み、どのケースが落ちたかが明確になる。

```go
func TestE1RM(t *testing.T) {
    tests := []struct {
        name              string
        weight, reps, rir float64
        want              float64
    }{
        {"1RM 100kg相当", 75, 8, 2, 100.0},
        {"RIRが結果を変える", 80, 8, 0, 101.33},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel()
            got := E1RM(tt.weight, tt.reps, tt.rir)
            if math.Abs(got-tt.want) > 0.01 {
                t.Errorf("E1RM() = %v, want %v", got, tt.want)
            }
        })
    }
}
```

移植時は **Phase 0 の Python 実装（`reference/analysis/tests/`）のテストケースを先に Go へ書き写す**。これがそのまま Red になり、[ADR-0011](docs/adr/0011-go-analytics.md) の「出力が一致すること」という受け入れ条件を満たす手順になる。

## 開発フロー

- `main` に直接コミットしない。ブランチを切る（`feat/T-01-workout-logging`）
- ブランチ名には**要件ID**を含める（[docs/01-要件定義.md](docs/01-要件定義.md) の T-01 / N-02 / A-14 など）
- Conventional Commits（説明文は日本語）: `feat(api): セット記録のエンドポイントを追加`
- 1 PR = 1つの変更。差分が300行を超えたら分割を検討する
- 詳細は [CONTRIBUTING.md](CONTRIBUTING.md)

### issue と PR の運用

**作業は必ず issue → ブランチ → PR の順で進める。** 追加機能や修正が必要になった時点で、実装前に issue を作る。

#### 1. issue を作る

```bash
gh issue create \
  --title "[T-02] 前回値のデフォルト表示" \
  --label feature \
  --body "$(cat <<'BODY'
## 要件ID
T-02

## 概要
前回同種目の重量・レップ・RIR をデフォルト表示する。

## 受け入れ条件
- [ ] 前回の値が初期表示される
- [ ] 前回記録がない種目では空欄になる
- [ ] 記録が 3秒以内・2タップ以内で完了する
BODY
)"
```

**受け入れ条件は必ず書く。** PR でこれを転記してチェックする。

大きい機能は親issueを立て、本文にタスクリスト（`- [ ] #12`）で子issueを並べる。

#### 2. ブランチを切って実装

```bash
git switch -c feat/T-02-default-previous-values
```

TDD で進める（上記）。**テストを先に書き、失敗を確認してから実装する。**

#### 3. PR を作る

```bash
gh pr create --fill --body "$(cat <<'BODY'
## 概要
...

Closes #12

## 受け入れ条件
- [x] 前回の値が初期表示される
...
BODY
)"
```

#### 4. CI の完了を待ってマージする

```bash
gh pr checks --watch              # CI の完了を待つ
gh pr merge --squash --delete-branch
```

**CI が緑になるまで待つ。** 待たずにマージしない。

### マージ前に必ず確認を求めるケース

以下に該当する場合は、**マージせず一度報告する**。

| ケース | 理由 |
|---|---|
| **CI が赤い** | 原因を調べて報告する。CI を無視してマージしない |
| **DB マイグレーションを含む** | 本番スキーマが変わる。**マージすると `deploy-api` が自動適用する**（[ADR-0016](docs/adr/0016-auto-migrate-on-deploy.md)）ので、戻せない変更が無人で流れる |
| **ADR を追加・変更した** | 設計判断なので、内容の合意を取る |
| **`infra/` を変更した** | `terraform apply` が必要になる |
| **`private/` に関わる変更** | 個人データの扱いが変わる |
| **受け入れ条件を満たしていない項目がある** | 未完了のままマージしない |

これら以外（機能追加・バグ修正・ドキュメント更新・リファクタ）は、**CI が緑なら確認なしでマージしてよい**。

### 設計判断は ADR に残す

トレードオフのある決定、覆すコストの高い決定は `docs/adr/` に記録する。**全ての決定に書くわけではない**（自明な選択や簡単に覆せる決定には書かない）。判断基準は [docs/adr/README.md](docs/adr/README.md)。

## スキーマ変更の手順

型は `openapi.yaml` が唯一の正（[ADR-0007](docs/adr/0007-openapi-schema-driven.md)）。**Go / TypeScript / Swift の型を手で書かない。**

1. `openapi.yaml` を編集
2. コード生成を実行（Go: `cd api && go generate ./...` / TS: `cd web && pnpm gen`。Swift は Phase 2）
3. 生成物をコミット（CI が最新性を検証する）
4. DB スキーマが変わるなら `api/migrations/` にマイグレーションを追加

### テーブルを足したら RLS を有効にする

```sql
alter table public.<新しいテーブル> enable row level security;
```

**Supabase は Postgres の前に PostgREST を自動で立てる。** Go API を通らない別の入口で、
`public` スキーマが公開対象になる。`000012` で権限は既定で剥がしてあるので**これを
忘れても即座には漏れない**が、二重に守るために付ける（[#150](https://github.com/Kenyan822/physique-analytics/issues/150)）。

ポリシーは作らない。RLS が有効でポリシーが無いテーブルは、素通りできないロールから
「行が無い」ように見える。アプリは `postgres`（`rolbypassrls`）で繋ぐので影響を受けない。

付け忘れは Supabase の security advisor が拾う（MCP の `get_advisors`）。

enum の値が日本語のときは `x-enum-varnames` で定数名を明示する。
書かないと生成される Go の定数が `N1` `N2` … になり読めなくなる。

## よく使うコマンド

```bash
# ローカル環境（Postgres）
docker compose up -d
docker compose down

# インフラ（Terraform）
cd infra && terraform plan
cd infra && terraform apply      # 手動実行。CI では plan のみ

# Web（Next.js）
cd web && pnpm install
cd web && pnpm dev
cd web && pnpm gen          # openapi.yaml → lib/api/schema.gen.ts
cd web && pnpm test
cd web && pnpm typecheck

# API（Go）
cd api && go generate ./...                    # openapi.yaml → gen/openapi/
cd api && go test ./... -race -cover
cd api && go test ./internal/analytics -run TestE1RM -v   # 単一テスト
cd api && golangci-lint run ./...

# API をローカルで起動（先に docker compose up -d が必要）
cd api && DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' go run ./cmd/server

# 本番と同じコンテナで動かす
docker compose --profile full up api

# DB マイグレーション（golang-migrate の公式イメージ）
docker compose run --rm migrate up
docker compose run --rm migrate down 1
docker compose run --rm migrate version

# iOS（Swift）
cd ios && swift test              # ロジックのテスト（Xcode 不要）
cd ios && xcodebuild -project Physique.xcodeproj -scheme Physique \
  -destination 'platform=iOS Simulator,name=iPhone 17 Pro' build

# リファレンス実装（Phase 0 の Python プロトタイプ。Go 移植の検証基準）
cd reference && pytest analysis/tests -v
cd reference && python3 analysis/analyze.py \
  --data-dir ../data/sample --config ../config.example.json --no-write

# 自分のデータで週次レポート（実データは private/）
cd reference && python3 analysis/analyze.py
```

## 構成

| ディレクトリ | 内容 |
|---|---|
| `api/` | Go。API サーバ・MCP サーバー・**分析ロジックの正**（[ADR-0011](docs/adr/0011-go-analytics.md)） |
| `web/` | Next.js。入力と最小限のサマリ表示 |
| `ios/` | Swift。ジムでの高速入力と HealthKit 連携 |
| `reference/analysis/` | Phase 0 プロトタイプ（Python）。**Go 移植の検証基準**。本番経路からは外れている |
| `docs/` | 要件・データモデル・分析ロジック・アーキテクチャ・インフラ |
| `docs/adr/` | 設計判断の記録 |
| `docs/go/` | **Go の学び**（公開する。上記ルール） |
| `docs/swift/` | **Swift / iOS の学びと仕組み**（公開する） |
| `private/learning/` | **個人の学習メモ**（公開しない）。サービスや概念の理解用 |
| `infra/` | Terraform。**シークレットの値は state に入れない**（[ADR-0012](docs/adr/0012-terraform.md)） |
| `private/` | 個人データ。`.gitignore` |
