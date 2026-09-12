# ADR-0007: REST + OpenAPI でスキーマ駆動にする

- Status: Accepted
- Date: 2026-09-12

## Context

[ADR-0006](0006-go-backend.md) でバックエンドを Go にしたことにより、**同じデータ構造を Go・TypeScript・Swift の3言語で定義する**必要が生じた。

```go
type DailyLog struct {
    Date     string  `json:"date"`
    WeightKg float64 `json:"weight_kg"`
}
```
```typescript
type DailyLog = { date: string; weight_kg: number }   // 手で書くと乖離する
```

手書きで管理すると、スキーマ変更のたびに3箇所を直す必要があり、**1箇所でも忘れると実行時に壊れる**。型チェックはコンパイル時に通ってしまうため、テストで拾えない限り本番で顕在化する。

## Decision

**OpenAPI をスキーマの唯一の正とし、3言語のコードを生成する。**

```
openapi.yaml                          ← 唯一の正
    ├─ oapi-codegen               → Go のサーバインターフェース + 型
    ├─ openapi-typescript         → TypeScript の型
    └─ swift-openapi-generator    → Swift のクライアント
```

スキーマ変更時は `openapi.yaml` のみを編集し、生成コマンドを実行する。**生成物をコミットし、CI で「生成物が最新か」を検証する**（差分があれば失敗させる）。

## Alternatives considered

**A. Connect (Protobuf)**
却下。技術的には優れており、以下の利点がある。

- Protobuf による厳密な型（フィールド番号での互換性管理）
- gRPC 互換でありながら HTTP/1.1 + JSON でも喋れるため、ブラウザから直接呼べて curl でデバッグできる
- Go 製で Go との相性が最良

それでも却下したのは、**ツールの成熟度と既存知識の活用を優先したため**。REST/OpenAPI は Postman などの既存ツールが使え、周辺情報も多い。バックエンド言語（Go）とインフラ（Cloud Run / Supabase）で既に新しい要素を2つ抱えており、**同時に学ぶ対象を増やすと Phase 1 の完了が遠のく**。

Connect は将来的に再検討する余地がある。その場合は本 ADR を Superseded とする。

**B. 素の gRPC**
却下。ブラウザから直接呼べず（gRPC-Web プロキシが必要）、curl でデバッグできない。このプロジェクトの通信頻度は1日数十リクエスト程度であり、**gRPC の性能上の優位が効く場面がない**。

**C. スキーマを持たず、型を手で書く**
却下。3言語での乖離が避けられない。乖離は実行時エラーとして現れるため、検出が遅れる。

## Consequences

**良い影響**

- **3言語の型が構造的に一致する。** 手書きしないため乖離しない
- **API 仕様がドキュメントとして機能する。** Swagger UI で動作確認でき、仕様書とコードが乖離しない
- REST なので、curl・Postman・ブラウザでそのまま検証できる
- モックサーバを生成でき、iOS 開発を API 完成前に始められる

**悪い影響 / 受け入れるコスト**

- **`openapi.yaml` の記述が冗長。** YAML の表現力に制約があり、複雑な型（union、条件付き必須）を書きづらい
- **生成物をコミットする運用が必要。** CI で最新性を検証する仕組みを入れないと、生成忘れが起きる
- Protobuf に比べて型の厳密さで劣る（フィールドの追加削除に対する互換性管理が弱い）
- Connect を選んだ場合に比べ、学習として得られるものは少ない
