package analytics

import "time"

// 日次データから判定の入力を組み立てる（要件 A-07 / A-08 の前処理）。
//
// 窓の切り方は reference/analysis/analyze.py の window() と同じで、
// `asof - days < date <= asof`。境界をずらすと、同じデータでも
// 検知の有無が変わる。

const (
	// TrendWindowDays は体重トレンドの回帰窓。
	// 短いと水分変動に振られ、長いと停滞の検出が遅れる。
	TrendWindowDays = 21

	// IntakeCVWindowDays は摂取の安定度を見る窓。
	IntakeCVWindowDays = 14

	// RecentWindowDays は「直近」の窓。7日平均と欠損の判定に使う。
	RecentWindowDays = 7

	// BaselineWindowDays は HRV・安静時心拍の基準となる窓。
	// 絶対値に個人差が大きいので、自分の30日平均を基準にする。
	BaselineWindowDays = 30
)

// DailyPoint は1日分の記録。集計に使う値だけを持つ。
//
// すべて *float64 なのは、「測っていない」を 0 と区別するため。
// Apple Watch を着けていない日、体重を測り忘れた日が普通にある。
type DailyPoint struct {
	Date         time.Time
	WeightKg     *float64
	BodyfatPct   *float64
	Kcal         *float64
	SleepH       *float64
	Steps        *float64
	Fatigue      *float64
	HrvMs        *float64
	RestingHr    *float64
	DeepSleepMin *float64
}

// WeeklyStat は週次レポートに出す集計値（要件 A-01 / A-06）。
type WeeklyStat struct {
	WeightKg7dAvg   *float64
	BodyfatPct7dAvg *float64
	// MeanKcal21d は TDEE の逆算に使う平均摂取
	MeanKcal21d *float64
	// WeightSlopeKgWeek は21日窓の体重トレンド
	WeightSlopeKgWeek *float64
	// KcalDays21d は21日窓で摂取を記録した日数。TDEE を出してよいかの判断に使う
	KcalDays21d int
}

// WeeklyStats は週次の集計値を返す。
func WeeklyStats(points []DailyPoint, asof time.Time) WeeklyStat {
	recent := within(points, asof, RecentWindowDays)
	trend := within(points, asof, TrendWindowDays)
	kcal21 := pick(trend, func(p DailyPoint) *float64 { return p.Kcal })

	return WeeklyStat{
		WeightKg7dAvg:     Mean(pick(recent, func(p DailyPoint) *float64 { return p.WeightKg })),
		BodyfatPct7dAvg:   Mean(pick(recent, func(p DailyPoint) *float64 { return p.BodyfatPct })),
		MeanKcal21d:       Mean(kcal21),
		WeightSlopeKgWeek: weightSlope(trend, asof),
		KcalDays21d:       len(kcal21),
	}
}

// BuildStallInput は日次データを停滞検知の入力に畳む。
//
// e1RMSlopeKgWeek は種目ごとの回帰（ExerciseHistory）から別に求めるので、
// ここでは受け取るだけにしている。
func BuildStallInput(points []DailyPoint, asof time.Time, goalKgPerWeek float64, e1rmSlope *float64) StallInput {
	recent := within(points, asof, RecentWindowDays)
	baseline := within(points, asof, BaselineWindowDays)
	// 前週の歩数は asof を1週ずらした窓で取る
	prevWeek := within(points, asof.AddDate(0, 0, -RecentWindowDays), RecentWindowDays)

	avg := func(ps []DailyPoint, get func(DailyPoint) *float64) *float64 {
		return Mean(pick(ps, get))
	}

	return StallInput{
		GoalKgPerWeek:     goalKgPerWeek,
		WeightSlopeKgWeek: weightSlope(within(points, asof, TrendWindowDays), asof),
		KcalCVPct: CVPct(pick(within(points, asof, IntakeCVWindowDays),
			func(p DailyPoint) *float64 { return p.Kcal })),
		E1RMSlopeKgWeek: e1rmSlope,

		Fatigue7dAvg:      avg(recent, func(p DailyPoint) *float64 { return p.Fatigue }),
		SleepH7dAvg:       avg(recent, func(p DailyPoint) *float64 { return p.SleepH }),
		Hrv7dAvg:          avg(recent, func(p DailyPoint) *float64 { return p.HrvMs }),
		Hrv30dAvg:         avg(baseline, func(p DailyPoint) *float64 { return p.HrvMs }),
		RestingHr7dAvg:    avg(recent, func(p DailyPoint) *float64 { return p.RestingHr }),
		RestingHr30dAvg:   avg(baseline, func(p DailyPoint) *float64 { return p.RestingHr }),
		DeepSleepMin7dAvg: avg(recent, func(p DailyPoint) *float64 { return p.DeepSleepMin }),
		Steps7dAvg:        avg(recent, func(p DailyPoint) *float64 { return p.Steps }),
		StepsPrev7dAvg:    avg(prevWeek, func(p DailyPoint) *float64 { return p.Steps }),

		MissingWeightDays: MissingDays(RecentWindowDays,
			len(pick(recent, func(p DailyPoint) *float64 { return p.WeightKg }))),
		MissingKcalDays: MissingDays(RecentWindowDays,
			len(pick(recent, func(p DailyPoint) *float64 { return p.Kcal }))),
	}
}

// weightSlope は体重の傾き（kg/週）を返す。
//
// x は asof からの経過日数（負）。記録の無い日は行ごと欠けるので、
// 等間隔を前提にせず日付から x を作る。
func weightSlope(points []DailyPoint, asof time.Time) *float64 {
	days := make([]float64, 0, len(points))
	values := make([]float64, 0, len(points))
	for _, p := range points {
		if p.WeightKg == nil {
			continue
		}
		days = append(days, p.Date.Sub(asof).Hours()/24)
		values = append(values, *p.WeightKg)
	}

	return SlopePerWeek(days, values)
}

// InWindow は date が `asof - days < date <= asof` に入るかを返す。
//
// **左端を含めない。** reference/analysis/analyze.py の window() と同じ規則で、
// ここがずれると同じデータでも傾きが変わる（実際に e1RM の傾きが
// -0.84kg/週 から -0.31kg/週 になり、検知の文面が変わった）。
//
// 日次以外の系列（種目ごとの e1RM など）でも同じ規則を使うため公開している。
func InWindow(date, asof time.Time, days int) bool {
	return date.After(asof.AddDate(0, 0, -days)) && !date.After(asof)
}

// within は窓に入る点だけを返す。
func within(points []DailyPoint, asof time.Time, days int) []DailyPoint {
	out := make([]DailyPoint, 0, len(points))
	for _, p := range points {
		if InWindow(p.Date, asof, days) {
			out = append(out, p)
		}
	}

	return out
}

// pick は nil でない値だけを取り出す。
func pick(points []DailyPoint, get func(DailyPoint) *float64) []float64 {
	out := make([]float64, 0, len(points))
	for _, p := range points {
		if v := get(p); v != nil {
			out = append(out, *v)
		}
	}

	return out
}
