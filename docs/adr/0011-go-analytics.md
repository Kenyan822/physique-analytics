# ADR-0011: 分析ロジックを Go に統一する

- Status: Accepted
- Date: 2026-09-12
- Amends: [ADR-0003](0003-build-own-analytics.md)（「Python 実装を分析の正とする」部分）

## Context

[ADR-0003](0003-build-own-analytics.md) では、分析ロジックを Python で実装し、**Python 実装を「分析の正」とする**方針を定めた。当時のバックエンドは Next.js Route Handlers であり、分析は別レイヤーとして切り離す前提だった。

しかしその後2つの決定により前提が変わった。

- [ADR-0006](0006-go-backend.md): バックエンドを Go にした
- [ADR-0010](0010-mcp-over-analysis-ui.md): MCP サーバーを Phase 1 に含め、分析の主経路にした

この状態で Python を維持すると、**分析ロジックが Go と Python の2言語に存在する**ことになる。MCP サーバーが Python なら API とは別プロセス・別デプロイになり、DB アクセス層も二重に持つ必要がある。

## Decision

**分析ロジックを Go に統一する。MCP サーバーも Go で実装する。**

既存の Python 実装は削除せず、`reference/` に移して**「Phase 0 のプロトタイプ兼、Go 移植の検証基準」**と位置づける。

```
api/
  internal/analytics/    ← 分析ロジックの正（Go）
  cmd/server/            ← API サーバ
  cmd/mcp/               ← MCP サーバー
reference/
  analysis/              ← Phase 0 のプロトタイプ（Python）
                            移植の検証基準として保持する
```

**Go 実装の受け入れ条件は「同一データに対して `reference/analysis/analyze.py` と出力が一致すること」とする。**

## Alternatives considered

**A. Python を正のまま維持し、MCP も Python で実装**
却下。API（Go）と MCP（Python）で DB アクセス層・認証・スキーマの解釈が二重になる。スキーマ変更のたびに両方を直す必要があり、[ADR-0007](0007-openapi-schema-driven.md) で型の二重管理を排除した判断と矛盾する。

**B. Python 実装を完全に削除する**
却下。23件のテストを伴う実装は**移植の検証基準として価値がある**。「Go の出力が Python と一致すること」という客観的な受け入れ条件を、他の方法で用意するのは難しい。また、なぜこの分析ロジック（TDEE の逆算、Epley + RIR補正、部位別 MEV/MRV）に至ったかの検証過程が失われる。

移植完了後も削除しない。ただし**本番の実行経路からは外す**。

## Consequences

**良い影響**

- **分析ロジックが1箇所に集約される。** API と MCP が同じパッケージ（`internal/analytics`）を共有し、DB アクセス層も1つで済む
- MCP サーバーが API と同じバイナリ・同じデプロイに乗る
- 数値計算の型安全性が得られる（[ADR-0006](0006-go-backend.md) の選定理由の1つ）
- **移植の正しさを機械的に検証できる。** 同一のサンプルデータに対して両実装の出力を比較すればよい

**悪い影響 / 受け入れるコスト**

- **移植の工数が発生する。** 線形回帰・移動平均・統計処理は Python（pandas / numpy）の方が記述量が少ない。Go では自前実装または gonum を使うことになり、コード量は増える
- **新しい分析仮説を試す速度が落ちる。** Python なら数行で試せる集計が、Go ではビルドを挟む。ただし MCP 経由で Claude Code に依頼する運用が主になるため（[ADR-0010](0010-mcp-over-analysis-ui.md)）、影響は限定的
- 移植が完了するまでの期間、実質的に2実装が存在する

## Notes

**CSV エクスポートは維持する。** `reference/analysis/` が動く状態を保つことで、Go 実装にバグがあった場合の比較対象として機能し続ける。[ADR-0002](0002-separate-personal-data.md) の通り CSV はバックアップの正でもあるため、この形式は他の理由でも必要。
