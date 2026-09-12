# 開発ガイド

## なぜ1人開発でPRベースなのか

このプロジェクトは現在1人で開発している。それでも issue / PR / CI を通すのは、以下の理由による。

1. **変更の理由を後から追えるようにするため。** コードは「何をしているか」は語るが「なぜそうしたか」「何を捨てたか」は語らない。issue と PR がその記録になる
2. **CI を必ず通すため。** ローカルでの「たぶん動く」を排除する
3. **他者が参加できる状態を保つため。** 参加者が現れてから整備するのでは遅い

ただし**形式のための形式は置かない**。レビュー承認の必須化やブランチ保護の厳格な設定は、1人では機能しないため導入していない。

## ブランチ戦略

**GitHub Flow** を採用する（`main` + 作業ブランチ）。1人開発に git-flow は重すぎるため。

- `main` は常にデプロイ可能な状態を保つ
- `main` に直接コミットしない
- 作業ブランチ名は **要件IDを含める**

```
feat/T-01-workout-logging
fix/A-04-e1rm-high-reps
chore/ci-setup
docs/adr-0003
```

要件IDは [docs/01-要件定義.md](docs/01-要件定義.md) の機能ID（T-01, N-02, A-14 など）を指す。

## コミットメッセージ

**Conventional Commits** に従う。説明文は日本語でよい。

```
<type>(<scope>): <説明>

feat(ios): セット記録画面に前回値のデフォルト表示を追加
fix(analysis): 限界12レップ超のセットがe1RM計算に混入する問題を修正
docs(adr): ADR-0003 データ分離の決定を追加
chore(ci): GitHub Actions に mypy を追加
test(analysis): TDEE推定のテストを追加
refactor(web): 週次レポートのデータ取得をServer Componentへ移動
```

| type | 用途 |
|---|---|
| `feat` | 機能追加 |
| `fix` | バグ修正 |
| `docs` | ドキュメントのみ |
| `test` | テストの追加・修正 |
| `refactor` | 挙動を変えない内部変更 |
| `chore` | ビルド・CI・依存関係 |
| `perf` | パフォーマンス改善 |

scope は `ios` / `web` / `analysis` / `ci` / `adr` など。

## issue の粒度

**1 issue = 1 PR = 半日〜2日で終わる単位。** PRの差分が300行を超えたら分割のサイン。

機能単位（要件ID）は大きすぎるため、親issueを立てて子issueに分割する。

```
#1 [T-01] トレーニング記録の基本フロー     ← 親issue（本文にタスクリスト）
  ├── #2 SwiftData スキーマ定義
  ├── #3 種目マスタ58件のシード
  ├── #4 セット入力UI
  ├── #5 前回値のデフォルト表示
  └── #6 インターバルタイマー
```

親issueの本文に `- [ ] #2` の形式でタスクリストを書くと、GitHub が進捗を自動でトラッキングする。

Milestone は Phase（[docs/01-要件定義.md](docs/01-要件定義.md) §8）に対応させる。

## Pull Request

- 1 PR = 1つの変更。無関係な変更を混ぜない
- 本文で `Closes #12` と issue を閉じる
- **受け入れ条件を満たしたことを本文に書く**（テンプレート参照）
- CI が緑になってからマージ
- マージは squash merge（履歴を1コミットにまとめる）

## テスト — TDD で進める

**実装を書く前にテストを書く。** Red → Green → Refactor。

1. **Red**: 失敗するテストを書き、**実際に失敗することを確認する**
2. **Green**: テストが通る最小の実装をする
3. **Refactor**: テストが通ったまま整える

**テストが最初から通ってしまう場合、そのテストは何も検証していない。** 実装前に必ず実行し、意図した理由で失敗することを確認する。

### 適用範囲

| 対象 | TDD |
|---|---|
| 分析ロジック（`api/internal/analytics`） | **必須** |
| API ハンドラ | **必須** |
| DB アクセス層 | **必須** |
| UI（Web / iOS） | 任意 |

分析ロジックは TDEE推定・e1RM計算・MEV/MRV判定の仕様が [docs/03-分析ロジック.md](docs/03-分析ロジック.md) に明文化されているため、**仕様がそのままテストケースになる**。TDD が最も機能する領域。

UI のテストは費用対効果が低いため必須としない。

### Go はテーブル駆動テストを基本とする

ケースの追加が1行で済み、どのケースが落ちたかが明確になる。`t.Parallel()` を付ける。

### 実行

```bash
cd api && go test ./... -race -cover        # Go（分析ロジックの正）
cd reference && pytest analysis/tests -v    # Phase 0 プロトタイプ（検証基準）
```

## 設計上の決定は ADR に残す

トレードオフのある決定、覆すコストの高い決定は [docs/adr/](docs/adr/) に記録する。

**全ての決定に書くわけではない。** 自明な選択（Prettier導入など）や簡単に覆せる決定には書かない。年に5〜10本が目安。

## パッケージマネージャ

**TypeScript / Node.js では pnpm を使う。** `npm` / `yarn` は使わない。

`package.json` の `packageManager` フィールドと `preinstall: only-allow pnpm` で強制しているため、`npm install` を実行するとエラーになる。理由は [06-技術選定.md](docs/06-技術選定.md) を参照。

## ローカル開発

```bash
# Python（分析）
pip install -r analysis/requirements.txt
python3 analysis/make_sample_data.py
python3 analysis/analyze.py --data-dir data/sample --config config.example.json --no-write

# Lint / Test
ruff check analysis/
mypy analysis/
pytest analysis/tests -v
```

個人の実データは `private/` 配下にあり、リポジトリには含まれない（[ADR-0002](docs/adr/0002-separate-personal-data.md)）。
