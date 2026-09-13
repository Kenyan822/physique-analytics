package analytics

import "fmt"

// 月次目標の生成（要件 P-02 / P-03）。
// 仕様は reference/analysis/make_monthly_plan.py。
//
// **追う順序は LBM → 体脂肪率 → 体重。** 体重は LBM と体脂肪率から
// 決まる従属変数であって、目標そのものではない。

// Baseline は計画の起点。
//
// P-03 のために毎回渡す。計画どおりに進まなかったときは、**実測値を
// 起点にして引き直す**。起点が変われば以降の全ての月がずれる。
type Baseline struct {
	WeightKg   float64
	BodyfatPct float64
	HeightCm   float64
}

// PlanBlock は計画のひと区切り。
//
// LBM の増減と、ブロック終了時点の体脂肪率で設計する。
// 体脂肪率はブロック内で等分に動かす。
type PlanBlock struct {
	Name   string
	Months int
	// LbmDeltaKgPerMonth は1ヶ月あたりの LBM の増減
	LbmDeltaKgPerMonth float64
	// BodyfatPctEnd はブロック終了時点の体脂肪率
	BodyfatPctEnd float64
}

// MonthlyTarget は各月末の到達目標。
type MonthlyTarget struct {
	// Month は YYYY-MM
	Month      string
	Phase      string
	LbmKg      float64
	BodyfatPct float64
	// WeightKg は LBM と体脂肪率から決まる従属変数
	WeightKg float64
	Ffmi     float64
}

// MonthlyTargets は起点とブロックから月次目標を組み立てる。
//
// startMonth は最初の月（YYYY-MM）。
func MonthlyTargets(base Baseline, blocks []PlanBlock, startMonth string) []MonthlyTarget {
	lbm := base.WeightKg * (1 - base.BodyfatPct/100)
	bf := base.BodyfatPct
	month := startMonth

	out := make([]MonthlyTarget, 0, 40)
	for _, b := range blocks {
		if b.Months <= 0 {
			continue
		}

		// ブロック内で体脂肪率を等分に動かす。月ごとに刻まないと、
		// 最終月だけ大きく動く不自然な計画になる
		step := (b.BodyfatPctEnd - bf) / float64(b.Months)

		for range b.Months {
			lbm += b.LbmDeltaKgPerMonth
			bf += step

			out = append(out, MonthlyTarget{
				Month:      month,
				Phase:      b.Name,
				LbmKg:      lbm,
				BodyfatPct: bf,
				WeightKg:   lbm / (1 - bf/100),
				Ffmi:       NormFFMI(lbm, base.HeightCm),
			})
			month = NextMonth(month)
		}
	}

	return out
}

// NextMonth は YYYY-MM を1ヶ月進める。読めない形式はそのまま返す。
func NextMonth(month string) string {
	var y, m int
	if _, err := fmt.Sscanf(month, "%d-%d", &y, &m); err != nil {
		return month
	}

	m++
	if m > 12 {
		y, m = y+1, 1
	}

	return fmt.Sprintf("%04d-%02d", y, m)
}
