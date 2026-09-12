# ADR-0006: バックエンドに Go を採用する

- Status: Accepted
- Date: 2026-09-12

## Context

API サーバの実装言語を決める必要があった。クライアントは iOS（Swift）、Web（Next.js）、Python 分析の3つで、いずれも API を介してデータにアクセスする。

当初のアーキテクチャでは Next.js の Route Handlers を API として使う想定だった。

**加えて、Go の実務経験を得ることも選定理由に含む。** 個人プロジェクトは本番で採用しづらい技術を試せる場であり、学習も正当な判断軸として扱う。ただし学習目的のみで選んだわけではなく、以下の設計上の利点がある。

## Decision

**API サーバを Go で実装し、Web（Next.js）から独立させる。**

```
api/          Go（API サーバ）
web/          Next.js（フロントエンドのみ）
ios/          Swift
analysis/     Python（分析ロジックの正）
```

## Alternatives considered

**A. Next.js Route Handlers（当初案）**
却下。理由は2つ。

1. **API が Web アプリに従属する。** クライアントは iOS・Web・Python の3つあり、そのうち1つ（Web）のフレームワークに API が組み込まれる構造は歪んでいる。Web を作り替えると API も巻き込まれる
2. 週次レポート生成や月次目標の再計算といった**バッチ処理を独立して動かしにくい**

ただし**型を TypeScript でフロントと共有できる**という明確な利点があり、これを失うコストは受け入れる（[ADR-0007](0007-openapi-schema-driven.md) で対処）。

**B. Python（FastAPI）**
却下。分析ロジックが既に Python にあるため一見合理的だが、
- Web/iOS と合わせて3言語になる点は Go と変わらない
- 分析ロジックは CSV 経由で疎結合にする方針（[ADR-0003](0003-build-own-analytics.md)）であり、同一プロセスに置く必要がない
- 実行環境のイメージサイズとコールドスタートで Go に劣る

## Consequences

**良い影響**

- **API が特定のフロントエンドに依存しない。** Web を作り替えても、iOS を作り直しても API は残る
- **バッチ処理を同じバイナリで動かせる。** 週次レポート生成・月次目標の再計算を Cloud Run Jobs や Cloud Scheduler から直接叩ける
- **数値計算の型安全性。** 移植する分析ロジックは単位を持つ数値（kg / kcal / kg per week / ms）だらけで、型の取り違えが事故になる。実際、Python 実装に mypy を導入した時点で型の曖昧さが4件検出された
- **デプロイが軽い。** distroless イメージで10MB台になり、Cloud Run のコールドスタートが速く無料枠に収まりやすい

**悪い影響 / 受け入れるコスト**

- **型の二重管理。** Go・TypeScript・Swift で同じデータ構造を定義することになる。手書きすると必ず乖離するため、スキーマ駆動で解決する（[ADR-0007](0007-openapi-schema-driven.md)）
- **デプロイ先が分かれる。** Web は Vercel、API は Cloud Run になり、CORS 設定と2箇所の監視が必要になる
- 1人開発における実装速度は、TypeScript で統一する場合より落ちる可能性がある
