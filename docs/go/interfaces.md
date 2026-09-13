# インターフェース

`api/internal` 全体で使っている、インターフェースまわりの判断。

## nil のポインタを interface に入れると非 nil になる

```go
type Deps struct {
	Contests ContestReader   // interface
}

var c *repository.Contest  // nil
deps := Deps{Contests: c}  // ← deps.Contests != nil になる

if d.Contests != nil {     // 通ってしまう
	d.Contests.Next(ctx, asof)  // panic
}
```

interface の値は **(型, 値)** の組で、型が入っていれば値が nil でも
interface 自体は nil ではない。

実際に踏んだ。MCP の `Deps.Contests` は `*repository.Contest` で、
テストが未設定（nil）のまま渡していたため、`weekly` 側の nil チェックを
すり抜けて segfault した。

**渡す側で分岐する。**

```go
deps := weekly.Deps{Plan: d.Plan, Series: d.Analysis}
if d.Contests != nil {
	deps.Contests = d.Contests
}
```

受け手で `reflect.ValueOf(x).IsNil()` を見る手もあるが、呼び出し側が
「入れない」方が意図が明確で、reflect も要らない。

## インターフェースは使う側のパッケージで定義する

```go
// internal/weekly
type PlanReader interface {
	Get(ctx context.Context) (openapi.Plan, error)
}
```

`repository.Plan` は `Get` 以外にもメソッドを持つが、`weekly` が要るのは
`Get` だけ。**使う側が必要な分だけ宣言する**と、テストのスタブが小さくなり、
`repository` に依存しないまま組み立てられる。

Java や C# の「実装側が interface を宣言して implements する」とは逆の向き。
Go の interface は暗黙的に満たされるので、これができる。

## 同じ計算を2か所に書かないための切り出し

`internal/weekly` は API と MCP の両方から使う。TDEE の逆算 → 推奨摂取 →
PFC の連なりを2か所に書くと、片方だけ直したときに値が食い違う。

**実際に1度やっている**（e1RM の窓の境界が API と reference でずれ、
傾きが -0.84 と -0.31 に割れた）。共有できる形を探す価値がある。
