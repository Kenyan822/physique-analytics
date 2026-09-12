# physique-analytics

[![CI](https://github.com/Kenyan822/physique-analytics/actions/workflows/ci.yml/badge.svg)](https://github.com/Kenyan822/physique-analytics/actions/workflows/ci.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](LICENSE)

トレーニング・栄養・体組成を統合して分析する、3年計画の肉体改造のための基盤。
**Web で記録・分析し、iOS はジムでの高速入力に特化する**構成。
API と分析ロジックは Go（[ADR-0006](docs/adr/0006-go-backend.md) / [ADR-0011](docs/adr/0011-go-analytics.md)）。

## 何を解決するか

既存のトレーニング記録アプリ（Hevy 等）と食事記録アプリ（MacroFactor 等）は入力体験が優れている一方、**構造的に解決できない問題**が2つある。

| 問題 | 内容 |
|---|---|
| **データの分断** | トレーニングと栄養が別アプリに分かれ、統合分析ができない。「炭水化物量とトン数の相関」「睡眠と翌日の推定1RM」を誰も見られない |
| **分析の浅さ** | 分析ロジックがブラックボックスで仮説を検証できない。部位別の週間セット数が適正範囲にあるかの判定、推定1RMの回帰による停滞検知が存在しないか浅い |

このプロジェクトは**分析レイヤーを自前で持つ**ことでこれを解決する（[ADR-0003](docs/adr/0003-build-own-analytics.md)）。

## 動かしてみる

### 記録してみる

```bash
docker compose up -d                       # Postgres 17
docker compose run --rm migrate up         # スキーマ + 種目マスタ49件

# API
cd api && DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' \
  AUTH_DISABLED=true go run ./cmd/server

# Web（別のターミナルで）
cd web && cp .env.example .env.local && pnpm install && pnpm dev
```

http://localhost:3000/log で記録できる。**種目を選ぶと前回の重量・レップ・RIR が
初期値に入る**（要件 T-02）。

### API を起動する

```bash
docker compose up -d                       # Postgres 17
docker compose run --rm migrate up         # スキーマ + 種目マスタ49件

cd api
DATABASE_URL='postgres://physique:dev@localhost:5432/physique?sslmode=disable' go run ./cmd/server
curl -s localhost:8080/health
```

### 分析を動かす

サンプルデータ（架空の記録55日分）が同梱されているので、clone してすぐ実行できる。
経路は2つあり、**Go が正**（[ADR-0011](docs/adr/0011-go-analytics.md)）。
Python は移植の検証基準として残してある。

#### MCP から（Go / 本番経路）

```bash
cd api && go build -o /tmp/physique-mcp ./cmd/mcp
claude mcp add physique -- /tmp/physique-mcp
```

環境変数は `DATABASE_URL`（Postgres の接続文字列）だけ。

主なツール。

| ツール | 返すもの |
|---|---|
| `weekly_report` | **週次レポート全文**（Markdown）。そのまま読ませる用 |
| `weekly_actions` | 同じ内容を構造化して返す。個々の数値を扱うとき |
| `weekly_volume` | 部位別の週間セット数と MEV/MRV 判定 |
| `exercise_progress` | 種目の推定1RM の推移と傾き |
| `query` | 読み取り専用の SQL |

計画の設定（目標ペース・PFC 係数）は **DB を正**とする（[ADR-0011](docs/adr/0011-go-analytics.md)）。
`config.json` から移すには `PHYSIQUE_CONFIG=... go run ./cmd/planimport` を一度実行する。

#### Python から（リファレンス実装）

```bash
cd reference
pip install -r analysis/requirements.txt
python3 analysis/make_sample_data.py
python3 analysis/analyze.py \
  --data-dir ../data/sample --config ../config.example.json \
  --asof 2026-10-31 --no-write
```

出力（抜粋）:

```markdown
## 1. 体重トレンド
| 7日平均体重        | 71.97 kg (n=7)   |
| トレンド(21日回帰)  | -0.81 kg/週      |
| 目標との乖離       | -0.05 kg/週      |
| 正規化FFMI        | 19.9             |

## 3. 推定TDEE と 来週の推奨摂取
| 平均摂取 (直近21日) | 2139 kcal        |
| 推定TDEE           | 3026 kcal        |
| 来週の推奨摂取      | 2190 kcal/日     |

## 5. 部位別 有効セット数 (直近7日)
| 部位     | セット | トン数    | MEV-MRV | 判定 |
| 胸       | 16    | 8474 kg  | 10-20   | ✓   |
| 大腿四頭  | 12    | 10932 kg | 10-20   | ✓   |
| ハム     | 8     | 5045 kg  | 8-16    | ✓   |

## 8. 今週のアクション
1. **筋力低下**: デッドリフト が -0.84 kg/週。減量ペースを -0.25kg/週 に緩める。
2. **筋力低下**: ミリタリープレス が -0.40 kg/週。減量ペースを -0.25kg/週 に緩める。
```

**最終成果物は「8. 今週のアクション」**。数値の羅列ではなく、来週何を変えるかが出ることをゴールにしている。

## 主要な分析

仕様は [docs/03-分析ロジック.md](docs/03-分析ロジック.md)。すべて純粋関数として実装し、テストしている。

| 分析 | 手法 |
|---|---|
| **TDEE の動的推定** | 21日窓の線形回帰 × 7700kcal/kg。計算式ではなく**実測から逆算**するため、代謝適応に毎週追随する |
| **推定1RM (e1RM)** | Epley + RIR補正。限界12レップ超は推定が崩れるため除外 |
| **部位別ボリューム** | 13部位の MEV/MRV 判定。肩を前部/中部/後部、背中を広背筋/僧帽筋に分割（一括では部位内の偏りが埋もれる） |
| **停滞検知** | 体重トレンド・摂取のばらつき・e1RM傾き・HRV の複合条件で判定 |
| **回復モニタリング** | Apple Watch の HRV / 安静時心拍 / 深睡眠。30日基準との乖離で判定 |

## アーキテクチャ

```
┌──────────────┐                ┌──────────────┐
│ Web (Next.js)│───────┐   ┌───▶│ Supabase     │
│ 入力 + サマリ  │       │   │    │ Postgres 17  │
│ オフライン対応  │       ▼   │    └──────────────┘
└──────────────┘ ┌──────────┴──┐        ▲
                 │  API (Go)   │        │ Supavisor
┌──────────────┐ │  Cloud Run  │────────┘ (6543)
│ iOS (Swift)  │▶│  + 分析ロジック│
│ ジムでの入力   │ │  JWT 検証     │
│ オフライン対応  │ └──────┬──────┘
└──────────────┘        │
                        │ MCP（手元から DB を直接読む）
                 ┌──────▼──────┐
                 │ Claude Code │  ← 分析はここから問い合わせる
                 └─────────────┘  （ADR-0010）
```

**Web と iOS はどちらもオフラインで記録でき、復帰時に送る**（要件 T-07）。
ジムは電波が悪く、送信に失敗した記録が消えると入力する意味が無くなるため。

**役割は「汎用 / ジム特化」で分ける**（[ADR-0009](docs/adr/0009-web-first-with-input.md)）。Web は入力も分析もできる汎用クライアント、**iOS の存在理由は「ジムでの入力速度」と「HealthKit 連携」**。開発は Web 先行。

Python 実装は移植後も破棄せず `reference/` に残す。**Go の分析結果が `reference/analysis/analyze.py` と一致することを受け入れ条件にする**（[ADR-0011](docs/adr/0011-go-analytics.md)）。

詳細は [docs/04-アーキテクチャ.md](docs/04-アーキテクチャ.md)。

## 設計上の決定

トレードオフのある決定は [ADR](docs/adr/) に記録している。

| # | 決定 |
|---|---|
| [0001](docs/adr/0001-monorepo.md) | monorepo 構成を採用する |
| [0002](docs/adr/0002-separate-personal-data.md) | 個人データをリポジトリから分離する |
| [0003](docs/adr/0003-build-own-analytics.md) | 分析ロジックを自作する |
| [0004](docs/adr/0004-ios-input-web-analysis.md) | iOS を入力、Web を分析に役割分離する |
| [0005](docs/adr/0005-features-not-implemented.md) | **バーコードスキャン・Watch入力・消費カロリー取込を実装しない** |
| [0006](docs/adr/0006-go-backend.md) | バックエンドに Go を採用する |
| [0007](docs/adr/0007-openapi-schema-driven.md) | REST + OpenAPI でスキーマ駆動にする |
| [0008](docs/adr/0008-r2-photo-storage.md) | 写真ストレージに Cloudflare R2 を採用する |
| [0009](docs/adr/0009-web-first-with-input.md) | Web を先行開発し、Web にも入力機能を持たせる |
| [0010](docs/adr/0010-mcp-over-analysis-ui.md) | 分析UIを作り込まず、MCP 経由での分析を主とする |
| [0011](docs/adr/0011-go-analytics.md) | 分析ロジックを Go に統一する |
| [0012](docs/adr/0012-terraform.md) | インフラを Terraform で管理する |
| [0013](docs/adr/0013-timezone-jst.md) | 日付は JST 固定で扱う |
| [0014](docs/adr/0014-sync-conflict-resolution.md) | 同期の競合は Last Write Wins + 論理削除で解決する |

0005 は「作らない決定」の記録。精度の低いデータをデータストアに持ち込まないことを優先している。

## 開発

```bash
# API (Go)
cd api
go generate ./...          # openapi.yaml → gen/openapi/
go test ./... -race -cover
golangci-lint run ./...

# リファレンス実装 (Python)
cd reference
pip install -r analysis/requirements-dev.txt
ruff check analysis/
mypy analysis/
pytest analysis/tests -v
```

開発フロー（ブランチ戦略・コミット規約・issue の粒度）は [CONTRIBUTING.md](CONTRIBUTING.md)。

**個人の実記録は `private/` 配下にあり、リポジトリには含まれない。** 体重・体組成・身体写真・血液検査結果を公開しないため（[ADR-0002](docs/adr/0002-separate-personal-data.md)）。

## ロードマップ

| Phase | 内容 | 状態 |
|---|---|---|
| **0** | 分析ロジック（Python リファレンス実装） | **完了** |
| **1** | **API (Go) + DB + Web + MCP**: 記録が回る状態 | **完了**（API 全24操作 / MCP / Web の入力とオフライン動作） |
| **2** | iOS: ジムでの高速入力 + HealthKit 連携 | **進行中**（入力画面が動作。HealthKit は未着手） |
| 3 | 食事記録・周囲長・写真・計画管理 | 未着手 |
| 4 | Watch 入力・位置情報サジェスト・相関分析 | 未着手 |

**分析UIは作らず、MCP 経由で Claude Code から分析する**（[ADR-0010](docs/adr/0010-mcp-over-analysis-ui.md)）。「睡眠6時間未満だった翌日の e1RM は平均どれくらい落ちるか」のような質問は事前に定義できず、固定の画面では構造的に対応できないため。Web に置くのは毎日見る少数の指標だけに留める。

各 Phase の受け入れ条件は [docs/01-要件定義.md](docs/01-要件定義.md) §8。

## ドキュメント

| | 内容 |
|---|---|
| [01-要件定義](docs/01-要件定義.md) | 機能要件（IDつき）・非機能要件・段階的リリース |
| [02-データモデル](docs/02-データモデル.md) | 何を測るか、測定条件、記録しないと決めたもの |
| [03-分析ロジック](docs/03-分析ロジック.md) | TDEE推定・e1RM・停滞検知の手法と閾値 |
| [04-アーキテクチャ](docs/04-アーキテクチャ.md) | 構成・技術選定・データフロー |
| [05-インフラ設計](docs/05-インフラ設計.md) | ホスティング・コスト試算・デプロイ・監視・障害対応 |
| [06-技術選定](docs/06-技術選定.md) | 採用バージョン・選定理由・更新方針・却下した選択肢 |
| [07-セットアップ](docs/07-セットアップ.md) | 外部サービスの作成手順（Supabase / GCP / 各種SaaS） |
| [ADR](docs/adr/) | 設計判断の記録 |
| [Go の学び](docs/go/) | 実装中に学んだことの記録（このプロジェクトは Go 学習を兼ねる） |

各コンポーネントの詳細は [api/README.md](api/README.md) / [web/README.md](web/README.md) /
[ios/README.md](ios/README.md)。

## License

[MIT](LICENSE)
