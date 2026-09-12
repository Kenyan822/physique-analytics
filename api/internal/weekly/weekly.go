// Package weekly は週次の判断に必要な値を1か所で組み立てる。
//
// **API（要件 N-05 の目標表示）と MCP（要件 A-08）で同じ計算を使うため**に
// 切り出してある。TDEE の逆算 → 推奨摂取 → PFC の連なりを2か所に書くと、
// 片方だけ直したときに値が食い違う（実際に窓の境界で1度やっている）。
package weekly

import (
	"context"
	"errors"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// TDEEMinDays は TDEE を出すのに必要な摂取の記録日数。
//
// 足りない状態で出すと、数日の記録漏れがそのまま「代謝が落ちた」に化ける。
const TDEEMinDays = 10

// PlanReader は計画の設定の読み取り。
type PlanReader interface {
	Get(ctx context.Context) (openapi.Plan, error)
	ListBlocks(ctx context.Context) ([]openapi.PlanBlock, error)
}

// ContestReader は次の大会の読み取り。
type ContestReader interface {
	Next(ctx context.Context, asof openapi_types.Date) (openapi.Contest, error)
}

// SeriesReader は日次記録の読み取り。
type SeriesReader interface {
	DailySeries(ctx context.Context, from, to openapi_types.Date) ([]analytics.DailyPoint, error)
}

// Deps は組み立てに必要な読み取り口。
type Deps struct {
	Plan   PlanReader
	Series SeriesReader
	// Contests は nil でもよい。大会を登録していなければカウントダウンが出ないだけ
	Contests ContestReader
}

// Summary は基準日における「今どうなっているか」。
type Summary struct {
	AsOf          time.Time
	Phase         string
	GoalKgPerWeek float64
	// InPlan は基準日が計画の期間内かどうか
	InPlan bool

	WeightKg7dAvg     *float64
	BodyfatPct7dAvg   *float64
	WeightSlopeKgWeek *float64
	TDEEKcal          *float64

	// Targets は摂取の目標。TDEE を推定できないときは nil
	Targets *Targets

	// Points は集計に使った日次記録。停滞検知（A-08）で再利用する
	Points []analytics.DailyPoint
	// Nutrition は適用した栄養パラメータ
	Nutrition analytics.NutritionConfig
	// Composition は LBM と正規化FFMI（要件 A-06）。体脂肪率が無ければ nil
	Composition *analytics.CompositionResult
	// Deviation は月次目標との乖離（要件 A-09）。目標を出せなければ nil
	Deviation *analytics.Deviation

	// Contest は次の大会と必要ペース（要件 A-10）。無ければ nil
	Contest *Countdown

	// Note は数字を出せなかった理由
	Note string
}

// Countdown は次の大会までの残りと、必要な減量ペース（要件 A-10）。
type Countdown struct {
	Contest   openapi.Contest
	WeeksLeft float64
	// Target は必要ペース。体重か体脂肪率が無いときは nil
	Target *analytics.ContestTarget
}

// Targets は1日の摂取目標（要件 A-02 / N-05）。
type Targets struct {
	KcalTarget float64
	ProteinG   float64
	FatG       float64
	CarbG      float64
	Phase      analytics.NutritionPhase
	// IntakeFloorHit / CarbBelowFloor は下限に達したか（要件 A-03）
	IntakeFloorHit bool
	CarbBelowFloor bool
}

// ErrNoPhases は計画のフェーズが1つも無いことを表す。
//
// メッセージに直し方を書く。MCP のエラーは Claude が読んで利用者に伝えるので、
// 「設定が無い」だけでは次の行動が決まらない。
var ErrNoPhases = errors.New(
	"フェーズが1つも登録されていない。Web の設定画面か PUT /v1/plan で登録する")

// Build は基準日の集計と目標を組み立てる。
func Build(ctx context.Context, d Deps, asof time.Time) (Summary, error) {
	plan, err := d.Plan.Get(ctx)
	if err != nil {
		return Summary{}, err
	}
	if len(plan.Phases) == 0 {
		return Summary{}, ErrNoPhases
	}

	out := Summary{AsOf: asof, Nutrition: NutritionConfig(plan.Nutrition)}
	out.GoalKgPerWeek, out.InPlan = repository.GoalAt(plan.Phases, asof)
	out.Phase = phaseName(plan, asof)
	if !out.InPlan {
		out.Note = "計画の期間外。目標ペースが決まらないので、判定は維持期として扱う"
	}

	// 30日分あれば HRV の基準まで取れる。前週の歩数のためにもう1週さかのぼる
	from := openapi_types.Date{
		Time: asof.AddDate(0, 0, -(analytics.BaselineWindowDays + analytics.RecentWindowDays)),
	}
	points, err := d.Series.DailySeries(ctx, from, openapi_types.Date{Time: asof})
	if err != nil {
		return Summary{}, err
	}
	out.Points = points

	stat := analytics.WeeklyStats(points, asof)
	out.WeightKg7dAvg = stat.WeightKg7dAvg
	out.BodyfatPct7dAvg = stat.BodyfatPct7dAvg
	out.WeightSlopeKgWeek = stat.WeightSlopeKgWeek

	out.withComposition(plan, asof)
	if err := out.withDeviation(ctx, d, plan, asof); err != nil {
		return Summary{}, err
	}
	if err := out.withContest(ctx, d, asof); err != nil {
		return Summary{}, err
	}

	// **推定できないのに数字を出さない。** 根拠の無い目標は判断を誤らせる
	if stat.MeanKcal21d == nil || stat.WeightSlopeKgWeek == nil || stat.KcalDays21d < TDEEMinDays {
		if out.Note == "" {
			out.Note = noteForMissingTDEE(stat.KcalDays21d)
		}

		return out, nil
	}

	tdee := analytics.EstimateTDEE(*stat.MeanKcal21d, *stat.WeightSlopeKgWeek)
	out.TDEEKcal = &tdee

	if stat.WeightKg7dAvg == nil {
		if out.Note == "" {
			out.Note = "体重の記録が無いため、摂取目標を出せない"
		}

		return out, nil
	}

	rec := analytics.RecommendedIntake(tdee, out.GoalKgPerWeek, *stat.WeightKg7dAvg)
	mt := analytics.MacroTargets(out.Nutrition, *stat.WeightKg7dAvg,
		stat.BodyfatPct7dAvg, out.GoalKgPerWeek, rec.RecommendedKcal)

	out.Targets = &Targets{
		KcalTarget:     rec.RecommendedKcal,
		ProteinG:       mt.ProteinG,
		FatG:           mt.FatG,
		CarbG:          mt.CarbG,
		Phase:          mt.Phase,
		IntakeFloorHit: rec.FloorHit,
		CarbBelowFloor: mt.CarbBelowFloor,
	}

	return out, nil
}

// withComposition は LBM と正規化FFMI を足す（要件 A-06）。
//
// 体脂肪率か身長が無ければ出さない。**推定値を混ぜない**（体組成計の
// 誤差は ±3〜5% あり、無い日を埋めると傾きが意味を失う）。
func (s *Summary) withComposition(plan openapi.Plan, _ time.Time) {
	if s.WeightKg7dAvg == nil || s.BodyfatPct7dAvg == nil || plan.HeightCm == nil {
		return
	}

	if c, ok := analytics.Composition(*s.WeightKg7dAvg, *s.BodyfatPct7dAvg, float64(*plan.HeightCm)); ok {
		s.Composition = &c
	}
}

// withDeviation は月次目標との乖離を足す（要件 A-09）。
//
// 起点かブロックが無ければ何もしない。計画を立てていないのは正常な状態。
func (s *Summary) withDeviation(ctx context.Context, d Deps, plan openapi.Plan, asof time.Time) error {
	if s.Composition == nil || plan.BaselineWeightKg == nil ||
		plan.BaselineBodyfatPct == nil || plan.BaselineMonth == nil || plan.HeightCm == nil {
		return nil
	}

	blocks, err := d.Plan.ListBlocks(ctx)
	if err != nil {
		return err
	}
	if len(blocks) == 0 {
		return nil
	}

	base := analytics.Baseline{
		WeightKg:   float64(*plan.BaselineWeightKg),
		BodyfatPct: float64(*plan.BaselineBodyfatPct),
		HeightCm:   float64(*plan.HeightCm),
	}
	targets := analytics.MonthlyTargets(base, toBlocks(blocks), *plan.BaselineMonth)

	target, ok := analytics.FindMonthlyTarget(targets, asof.Format("2006-01"))
	if !ok {
		return nil
	}

	dev := analytics.PlanDeviation(target, *s.Composition, *s.WeightKg7dAvg, *s.BodyfatPct7dAvg)
	s.Deviation = &dev

	return nil
}

// toBlocks は openapi のブロックを analytics が受け取る形にする。
func toBlocks(blocks []openapi.PlanBlock) []analytics.PlanBlock {
	out := make([]analytics.PlanBlock, 0, len(blocks))
	for _, b := range blocks {
		out = append(out, analytics.PlanBlock{
			Name:               b.Name,
			Months:             b.Months,
			LbmDeltaKgPerMonth: float64(b.LbmDeltaKgPerMonth),
			BodyfatPctEnd:      float64(b.BodyfatPctEnd),
		})
	}

	return out
}

// withContest は次の大会と必要ペースを足す（要件 A-10）。
//
// 大会を登録していなければ何もしない。計画に大会が無いのは正常な状態で、
// エラーにするようなことではない。
func (s *Summary) withContest(ctx context.Context, d Deps, asof time.Time) error {
	if d.Contests == nil {
		return nil
	}

	c, err := d.Contests.Next(ctx, openapi_types.Date{Time: asof})
	if errors.Is(err, repository.ErrNotFound) {
		return nil
	}
	if err != nil {
		return err
	}

	cd := &Countdown{Contest: c, WeeksLeft: analytics.WeeksUntil(asof, c.HeldOn.Time)}
	if s.WeightKg7dAvg != nil && s.BodyfatPct7dAvg != nil {
		if t, ok := analytics.ContestPace(*s.WeightKg7dAvg, *s.BodyfatPct7dAvg,
			float64(c.TargetBfPct), cd.WeeksLeft); ok {
			cd.Target = &t
		}
	}
	s.Contest = cd

	return nil
}

// ActionContext は停滞検知の文脈（要件 A-08）に渡す形を作る。
func (s Summary) ActionContext() analytics.ActionContext {
	ctx := analytics.ActionContext{GoalKgPerWeek: s.GoalKgPerWeek}
	if s.Deviation != nil && s.Deviation.LbmBehind {
		ctx.LbmBehindKg = s.Deviation.LbmKg
		ctx.LbmBehind = true
	}
	if s.Contest != nil && s.Contest.Target != nil && s.Contest.Target.TooFast {
		ctx.ContestPaceTooFast = true
		ctx.ContestWeeksLeft = s.Contest.WeeksLeft
		ctx.ContestPacePctPerWeek = s.Contest.Target.PacePctPerWeek
	}
	if s.Targets != nil {
		ctx.IntakeFloorHit = s.Targets.IntakeFloorHit
		ctx.CarbBelowFloor = s.Targets.CarbBelowFloor
		ctx.CarbTargetG = s.Targets.CarbG
	}

	return ctx
}

// NutritionConfig は openapi の設定を analytics が受け取る形にする。
func NutritionConfig(n openapi.NutritionSettings) analytics.NutritionConfig {
	macros := func(m openapi.MacroRatio) analytics.Macros {
		return analytics.Macros{
			ProteinGPerKg: float64(m.ProteinGPerKg),
			FatGPerKg:     float64(m.FatGPerKg),
		}
	}

	return analytics.NutritionConfig{
		Cut:                macros(n.Cut),
		DeepCut:            macros(n.DeepCut),
		Bulk:               macros(n.Bulk),
		DeepCutBfThreshold: float64(n.DeepCutBfThreshold),
		CarbMinG:           float64(n.CarbMinG),
	}
}

func phaseName(p openapi.Plan, asof time.Time) string {
	day := asof.Format(time.DateOnly)
	for _, ph := range p.Phases {
		if ph.StartsOn.Format(time.DateOnly) <= day && day <= ph.EndsOn.Format(time.DateOnly) {
			return ph.Name
		}
	}

	return ""
}

func noteForMissingTDEE(kcalDays int) string {
	return fmt.Sprintf("TDEE を推定できない（直近%d日で摂取の記録が%d日 / 必要%d日）。"+
		"摂取量の意思決定にはこれが要る", analytics.TrendWindowDays, kcalDays, TDEEMinDays)
}
