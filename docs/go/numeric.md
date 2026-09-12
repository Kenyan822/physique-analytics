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

## 閾値は名前付き定数にして、判定関数には集計済みの値だけ渡す

```go
const (
	PlateauSlopeKgWeek = 0.1
	FatigueMax7d       = 3.5
)

type StallInput struct {
	WeightSlopeKgWeek *float64   // 21日回帰の結果
	Fatigue7dAvg      *float64   // 平均を取るのは呼び出し側
}
```

判定関数の中で期間を切ったり平均を取ったりしない。そうすると
**閾値のテストに時系列データを組み立てる必要が出て**、仕様表
（docs/03-分析ロジック.md）との対応が読み取れなくなる。

集計と判定を分けておけば、テストは `Fatigue7dAvg: ptrF(3.6)` の1行で済む。

## 境界条件は仕様の日本語をそのまま写す

```go
// 仕様は「-0.3kg/週 以下」なので境界を含める
if *in.E1RMSlopeKgWeek <= StrengthDropKgWeek {
```

「以下」なら `<=`、「未満」なら `<`。書き分けを間違えても
ほとんどのテストは通ってしまうので、**境界そのものを1ケースとして書く**
（-0.3 で検知、-0.29 で検知しない）。

## `max` は組み込みなので自分で書かない

```go
missing := float64(max(in.MissingKcalDays, in.MissingWeightDays))
```

Go 1.21 から `min` / `max` が組み込みで、順序付きの型なら何でも使える。
`math.Max` は `float64` 専用なので、int の比較に使うとキャストが増える。

## 並べ替えは `slices.SortStableFunc` と「順序表」で書く

```go
var order = map[StallKind]int{
	StallMissingRecords: 0,
	StallHrvDrop:        0,
	StallStepsDrop:      0,
}

slices.SortStableFunc(out, func(a, b Action) int {
	if a.Priority != b.Priority {
		return int(a.Priority) - int(b.Priority)
	}

	return order[a.Kind] - order[b.Kind]
})
```

比較関数は `bool` ではなく **3値の int**（負/0/正）を返す。`sort.Slice` の
`less func(i, j int) bool` とはシグネチャが違うので混ぜない。

`SortStableFunc` にしているのは、順序表に無い種類（`order[k]` が 0 になる）が
複数あっても、入力順が保たれて出力が安定するため。`SortFunc` だと
同点の並びが実行ごとに変わり、テストが不安定になる。

## `iota` で優先度を作ると「小さいほど高い」が型で表せる

```go
const (
	PriorityBlocker ActionPriority = iota
	PriorityRecovery
	PriorityAdjust
	PriorityInfo
)
```

数値の大小がそのまま並び順になるので、比較関数に変換表を持たなくて済む。
定義の順序が仕様（記録 → 回復 → 調整）そのものになる点も読みやすい。

## 窓の境界は1か所に寄せる

```go
// InWindow は date が `asof - days < date <= asof` に入るかを返す。
func InWindow(date, asof time.Time, days int) bool {
	return date.After(asof.AddDate(0, 0, -days)) && !date.After(asof)
}
```

**同じ「42日窓」でも、左端を含めるかで結果が変わる。** 実際にやらかした:

| 窓 | 点数 | 傾き |
|---|---|---|
| `date >= asof-42`（左端を含む） | 7 | -0.31 kg/週 |
| `date > asof-42`（含まない） | 6 | **-0.84 kg/週** |

1点増えただけで傾きが 2.7倍 変わり、レポートの文面も変わった。
SQL 側は `>=` で書きたくなる（`between` が自然）ので、**取得は広めにして
判定は Go の1つの関数に通す**形にした。

## 導出できるものは保存しない

月次目標（要件 P-02）は、起点とブロックから**毎回計算する**。
月ごとの表を DB に持たない。

```go
func MonthlyTargets(base Baseline, blocks []PlanBlock, startMonth string) []MonthlyTarget
```

保存すると「引き直したつもりで古い表が残っている」が起きる。
要件 P-03（実測 LBM を起点に引き直す）がある以上、**起点は変わる前提**で、
変わったら全ての月がずれる。

保存すべきは**入力（起点とブロック）だけ**で、出力は関数にする。
37ヶ月ぶんの計算は一瞬なので、キャッシュする理由も無い。

## 「黙って別の値に落ちる」を作らない

```go
if sum.WeightKg7dAvg == nil || sum.BodyfatPct7dAvg == nil {
	return nil, "", "直近7日の体重または体脂肪率が無いため、実測から引き直せない", nil
}
```

`baseline=measured` を指定されて実測が無いとき、設定値へ落とすと
**どちらを見ているか分からないまま数字を読む**ことになる。

エラーにして「なぜ出せないか」を返す。判断に使う数字は、
出どころが曖昧なまま出す方が害が大きい。
