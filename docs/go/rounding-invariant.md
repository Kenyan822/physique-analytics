# 丸めても壊れない不変条件を、丸め方で守る

ざっくり入力（kcal だけ入れて P 20% / F 30% / C 50% に按分、#252）で学んだこと。

## 3つを別々に丸めると、再計算した kcal がずれる

```go
p := round1(k * 0.20 / 4)
f := round1(k * 0.30 / 9)   // ← これが原因
c := round1(k * 0.50 / 4)
// 4p + 9f + 4c は k から最大 0.85 kcal ずれ、四捨五入すると 1 ずれる日がある
```

画面は PFC から kcal を出し直す（Atwater 4/9/4）ので、「1000 と入れたのに 1001 と出る」。

## 判断: P と C を丸め、F は残りの kcal から逆算して丸める

```go
proteinG = round1(k * roughProteinShare / kcalPerGProtein)
carbG    = round1(k * roughCarbShare / kcalPerGCarb)
fatG     = math.Max(0, round1((k - proteinG*4 - carbG*4) / 9))
```

**状況**: 保存先が `numeric(6,1)` で、小数1桁に丸めるしかない。

**判断**: 丸め誤差を1か所（F）に集める。

**理由**: F の丸め誤差は最大 0.05g × 9 = 0.45 kcal。0.5 未満なので、`math.Round` で
kcal に戻すと**必ず元の値**になる。代償は F が比率から最大 0.1g 程度動くことで、
手で直す前提の按分には十分小さい。

## 不変条件は「全部の入力で」テストする

```go
for kcal := 0; kcal <= 10000; kcal++ {
	p, f, c := analytics.RoughMacros(kcal)
	if got := analytics.KcalFromMacros(float64(float32(p)), float64(float32(f)), float64(float32(c))); got != kcal {
		t.Fatalf(...)
	}
}
```

入力が有限（kcal は 0〜10000 の整数）なので、サンプルではなく**全数**で確かめられる。
境界値を選ぶより速くて確実。`float32` を通しているのは、API で `float32` として保存・返却するため
（`float32(33.3)` を `float64` に戻すと 33.29999…になる）。

## `math.Round` は 0.5 を絶対値が大きい方へ丸める

`math.Round(2.5) == 3`、`math.Round(-2.5) == -3`。Python の `round(2.5) == 2`（偶数丸め）と違う。
按分の値は非負なので問題ないが、**Python 実装との出力一致を見る移植では差が出る**。

## 負にならないようにする

`k` が小さいと、P と C を丸め上げた結果が `k` を超えて F が負になりうる。`math.Max(0, …)` で止め、
その場合も再計算した kcal が戻ることを全数テストで確かめている。
