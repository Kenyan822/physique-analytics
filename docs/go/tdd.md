# TDD の進め方（実際の流れ）

[CLAUDE.md](../../CLAUDE.md) に「Red → Green → Refactor」と書いてあるが、それだけでは
実際に手を動かすときに迷う。**このプロジェクトで実際にやった手順**を、
`/health` と `exercises` の実装を例に残す。

ルールの理屈ではなく、**どのコマンドをどの順で叩いたか**を書く。

---

## 全体の流れ

```
1. openapi.yaml を見て、何を保証すべきかを決める
2. テストを書く            ← 実装ファイルはまだ無い
3. go test → 失敗を見る    ← ここを飛ばさない
4. 通る最小の実装を書く
5. go test → 通る
6. 整える（テストが通ったまま）
7. golangci-lint / go vet
```

---

## 1. 何を保証すべきかは openapi.yaml から取る

型の正が `openapi.yaml` なので、**テストケースも仕様書から引く**。

```yaml
/v1/exercises:
  get:
    parameters:
      - $ref: "#/components/parameters/MuscleGroup"
      - $ref: "#/components/parameters/IncludeDeleted"   # 論理削除済みを含めるか。既定 false
```

ここから出てくるテストはこう:

- 絞り込みなしで全部返る
- `muscleGroup` で絞れる
- `includeDeleted` 未指定なら論理削除済みは返らない
- `includeDeleted=true` なら返る

**仕様に書いてあることがそのままテスト名になる。** 思いつきでケースを足すより、
仕様を1行ずつ潰す方が漏れがない。

---

## 2. テストを先に書く

`internal/repository/exercise_test.go` を作る。この時点で `exercise.go` は無い。

```go
func TestExerciseList_部位で絞れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewExercise(testdb.Begin(t))

	chest := openapi.Chest
	got, err := repo.List(ctx, repository.ExerciseFilter{MuscleGroup: &chest})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != 8 {
		t.Errorf("胸の件数 = %d, want 8", len(got))
	}
}
```

**存在しない API を呼ぶ形で書く。** `repository.NewExercise` も `ExerciseFilter` も
まだ無いが、「こう呼べたら書きやすい」という形を先に決める。
これが実質的な設計になる。

---

## 3. 失敗を見る（飛ばさない）

```console
$ go test ./internal/repository/
# github.com/Kenyan822/physique-analytics/api/internal/repository
internal/repository/exercise_test.go:8:2: no required module provides package .../internal/testdb
FAIL	github.com/Kenyan822/physique-analytics/api/internal/repository [setup failed]
```

Go では最初の Red は**コンパイルエラー**になることが多い。それでよい。

**確認したいのは「テストが実際に走って落ちる」こと。** 書いたつもりのテストが
実は空振りしていた、を防ぐ唯一の方法がこれ。ここを飛ばすと、後で
「通っているように見えるが何も検証していないテスト」が残る。

---

## 4. 通る最小の実装

`repository.go`（共通）と `exercise.go`（本体）を書く。

この段階で凝らない。**まず緑にする。**

---

## 5. 緑にならないときは、まず原因を切り分ける

実際にここで詰まった。

```console
$ go test ./internal/repository/ -v
--- FAIL: TestExerciseList_部位で絞れる (0.03s)
    exercise_test.go:39: 胸の件数 = 9, want 8
--- FAIL: TestExerciseList_シードされた種目が全部返る (0.03s)
    exercise_test.go:23: 件数 = 50, want 49
```

+1 されている。実装のバグではなく、**別のテストが挿入した行が見えていた**。
`t.Parallel()` で並行に走る3つのテストが同じ DB を共有していたため。

**テストが落ちたとき、疑う順序**:

1. テストの書き方（今回はこれ）
2. 実装
3. 仕様の理解

「実装が悪い」から入ると、テストの設計ミスを実装で回避する形になって余計に壊れる。

---

## 6. Refactor: テストをトランザクションで隔離する

`repository` が `*pgxpool.Pool` を直接受けていたのをインターフェースに変えた。

```go
// *pgxpool.Pool と pgx.Tx の両方が満たす
type DBTX interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
}
```

テスト側は**トランザクションを開いて最後に Rollback する**。

```go
func Begin(t *testing.T) pgx.Tx {
	tx, err := pool.Begin(ctx)
	...
	t.Cleanup(func() { tx.Rollback(context.Background()) })
	return tx
}
```

未コミットの変更は他のトランザクションから見えないので、並行しても干渉しない。
後始末も要らない（`delete` を書かなくてよくなった）。

```console
$ go test ./internal/repository/ -v
--- PASS: TestExerciseList_シードされた種目が全部返る (0.13s)
--- PASS: TestExerciseGet_存在しなければErrNotFound (0.13s)
--- PASS: TestExerciseList_部位で絞れる (0.14s)
--- PASS: TestExerciseGet_存在する種目を取れる (0.15s)
--- PASS: TestExerciseList_論理削除済みは既定で返らない (0.17s)
ok  	.../internal/repository	1.168s
```

**Refactor は「整える」だけでなく、テストが教えてくれた設計の問題を直す段階。**
インターフェースを挟む判断は、テストを書かなければ出てこなかった。

---

## 7. どの層で何をテストするか

同じことを2回テストしない。

| 層 | テスト対象 | 何を使うか |
|---|---|---|
| `repository` | SQL の正しさ。絞り込み、論理削除、部分インデックス | **本物の Postgres** |
| `handler` | パラメータの受け渡し、エラーの HTTP への変換 | **repository のスタブ** |

**`repository` でモックを使わない理由**: 確かめたいことの大半が SQL と DB の挙動そのもの。
`where deleted_at is null` が効いているか、`muscle_group` の enum が通るか、
部分ユニークインデックスが効くか —— これらはモックでは全部通ってしまう。

**`handler` で本物の DB を使わない理由**: ハンドラの仕事は変換だけ。
`ErrNotFound` を 404 + Problem にする、クエリパラメータを filter に詰める。
DB を立てる必要がない。

```go
// handler のテスト用スタブ。4行で済む
type stubExercises struct {
	items     []openapi.Exercise
	gotFilter *repository.ExerciseFilter
	getErr    error
}

func (s *stubExercises) List(_ context.Context, f repository.ExerciseFilter) ([]openapi.Exercise, error) {
	s.gotFilter = &f   // 何が渡ってきたかを記録して、後で assert する
	return s.items, nil
}
```

---

## 8. DB が無い手元で落ちないようにする

`testdb.Begin` は接続できなければ `t.Skip` する。

```go
if poolErr != nil {
	t.Skipf("テスト用 DB に接続できない（docker compose up -d で起動する）: %v", poolErr)
}
```

ただし **CI では Skip を許さない。** 許すと「DB を立て忘れたまま緑」になり、
DB のテストが実質存在しない状態に気づけない。

```yaml
- name: DB のテストが Skip されていないこと
  run: |
    go test ./internal/repository/ -v 2>&1 | tee /tmp/repo.log
    if grep -q -- '--- SKIP' /tmp/repo.log; then
      echo '::error::DB のテストが Skip された'
      exit 1
    fi
```

---

## 9. 最後に静的チェック

```console
$ go vet ./...
$ golangci-lint run ./...
0 issues.
```

テストが通ってから掛ける。順番を逆にすると、動くかどうか分からないコードの
スタイルを直すことになって時間が無駄になる。

---

## やらなかったこと

**ハンドラ層は先にテストを書かなかった。** `repository` を書いた勢いで
`exercise.go` を書いてから、テストを足した。

薄い変換層で設計の迷いが無かったのが理由だが、**TDD としては崩れている**。
`ErrNotFound` → 404 の変換はハンドラ固有のロジックなので、
本来は先にテストを書くべきだった。

記録として残しておく。次はここも先に書く。

---

## まとめ

1. テストケースは `openapi.yaml` から引く。思いつきで足さない
2. **失敗を必ず見る。** コンパイルエラーでもよい
3. テストが落ちたら、まずテストの書き方を疑う
4. Refactor はテストが教えてくれた設計の問題を直す段階
5. `repository` は本物の DB、`handler` はスタブ。同じことを2回テストしない
6. CI では Skip を許さない
