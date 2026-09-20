# Architecture Decision Records

トレードオフのある決定、覆すコストの高い決定を1決定1ファイルで記録する。

**全ての決定に書くわけではない。** 自明な選択や簡単に覆せる決定には書かない。書く基準は以下。

- 検討して**却下した案**がある
- 覆すコストが高い（DB・フレームワーク・データの持ち方）
- 「普通はこうするが、あえて違う」選択をした
- 後から見ると不自然に見えうる

| # | 決定 | Status |
|---|---|---|
| [0001](0001-monorepo.md) | monorepo 構成を採用する | Accepted |
| [0002](0002-separate-personal-data.md) | 個人データをリポジトリから分離する | Accepted |
| [0003](0003-build-own-analytics.md) | 分析ロジックを自作する | Accepted（一部を 0011 が修正） |
| [0004](0004-ios-input-web-analysis.md) | iOS を入力、Web を分析に役割分離する | ~~Superseded by 0009~~ |
| [0005](0005-features-not-implemented.md) | バーコードスキャン・Watch入力・消費カロリー取込を実装しない | Accepted |
| [0006](0006-go-backend.md) | バックエンドに Go を採用する | Accepted |
| [0007](0007-openapi-schema-driven.md) | REST + OpenAPI でスキーマ駆動にする | Accepted |
| [0008](0008-r2-photo-storage.md) | 写真ストレージに Cloudflare R2 を採用する | Accepted |
| [0009](0009-web-first-with-input.md) | **Web を先行開発し、Web にも入力機能を持たせる** | Accepted |
| [0010](0010-mcp-over-analysis-ui.md) | **分析UIを作り込まず、MCP 経由での分析を主とする** | Accepted |
| [0011](0011-go-analytics.md) | **分析ロジックを Go に統一する** | Accepted |
| [0012](0012-terraform.md) | **インフラを Terraform で管理する** | Accepted |
| [0013](0013-timezone-jst.md) | **日付は JST 固定で扱う** | Accepted |
| [0014](0014-sync-conflict-resolution.md) | **同期の競合は LWW + 論理削除で解決する** | Accepted |
| [0015](0015-plan-settings-in-db.md) | 計画の設定は DB を正とする | Accepted |
| [0016](0016-auto-migrate-on-deploy.md) | マイグレーションはデプロイの前段で自動適用する | Accepted |
| [0017](0017-food-master-with-parameters.md) | 食品マスタを持つ。ただし「引数つきの計算式」として持つ | Accepted |

形式は [Michael Nygard の ADR](https://cognitect.com/blog/2011/11/15/documenting-architecture-decisions) に準拠する。
