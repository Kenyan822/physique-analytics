# 数値の扱い

分析ロジック（`api/internal/analytics`）で数値を扱ったときの学び。

## 設定値を定数にするか引数にするかは「個人データか」で決める

```go
const FloorKcalPerKg = 24.0        // ○ 生理学的な定数。誰でも同じ

type NutritionConfig struct {      // ○ 個人の設定。呼び出し側が渡す
	Cut     Macros
	CarbMinG float64
}
```

`7700 kcal/kg`（脂肪1kgのカロリー）や `24 kcal/kg`（摂取の下限）は
定数にしてよいが、タンパク質の g/kg や体脂肪率の閾値は個人設定なので
パッケージに埋めない（ADR-0002 で public 側に実数値を書かない方針）。

**判断基準は「他人に配ってもよい値か」。** 配れるなら定数、配れないなら引数。

## `*float64` を「測っていない」の表現に使う

```go
func MacroTargets(cfg NutritionConfig, bw float64, bodyfatPct *float64, ...) MacroTarget {
	switch {
	case goalKgPerWeek > 0:
		phase = PhaseBulk
	case bodyfatPct != nil && *bodyfatPct < cfg.DeepCutBfThreshold:
		phase = PhaseDeepCut
	}
}
```

0 を「未測定」の代わりに使わない。体脂肪率 0% は測定値としてありえないが、
`0 < 13.0` は真になるので、**未測定が「絞れている」と誤判定される**。

Python では `None` とスカラーが同じ変数に入るので気にせず書けるが、
Go では型で分けるしかない。ここは Go の方が事故が起きにくい。

## 従属変数はクランプして返す

```go
carb := (intakeKcal - protein*4 - fat*9) / 4
if carb < 0 {
	carb = 0
}
```

炭水化物は「摂取枠の残り」なので、P と F だけで枠を超えると負になる。
式のまま返すと `-181g` という指示が出る。**式が破綻する入力を許すのではなく、
返す側で止める。**

このとき reference の Python 実装も同時に直した。ADR-0011 で
「出力が一致すること」を受け入れ条件にしているので、片方だけ直すと
検証基準として使えなくなる。
