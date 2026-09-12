package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/plan"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// keyExercises は e1RM の傾きを見る主要種目。
//
// 全種目を見ない。サイドレイズのような単関節種目は日による振れが大きく、
// 「筋力が落ちた」の判断材料にならない（reference の KEY_EXERCISES と同じ）。
var keyExercises = []string{
	"ベンチプレス", "スクワット", "デッドリフト", "ミリタリープレス", "荷重懸垂", "バーベルロー",
}

type weeklyActionsIn struct {
	AsOf string `json:"asOf,omitempty" jsonschema:"基準日 YYYY-MM-DD（JST）。省略時は今日"`
}

type actionOut struct {
	Kind     string `json:"kind"`
	Priority string `json:"priority"`
	Text     string `json:"text"`
}

type weeklyActionsOut struct {
	AsOf          string      `json:"asOf"`
	Phase         string      `json:"phase,omitempty"`
	GoalKgPerWeek *float64    `json:"goalKgPerWeek,omitempty"`
	TDEEKcal      *float64    `json:"tdeeKcal,omitempty"`
	IntakeKcal    *float64    `json:"intakeKcal,omitempty"`
	ProteinG      *float64    `json:"proteinG,omitempty"`
	FatG          *float64    `json:"fatG,omitempty"`
	CarbG         *float64    `json:"carbG,omitempty"`
	WeightKg      *float64    `json:"weightKg7dAvg,omitempty"`
	BodyfatPct    *float64    `json:"bodyfatPct7dAvg,omitempty"`
	SlopeKgWeek   *float64    `json:"weightSlopeKgPerWeek,omitempty"`
	Actions       []actionOut `json:"actions"`
	Note          string      `json:"note,omitempty"`
}

var priorityLabel = map[analytics.ActionPriority]string{
	analytics.PriorityBlocker:  "blocker",
	analytics.PriorityRecovery: "recovery",
	analytics.PriorityAdjust:   "adjust",
	analytics.PriorityInfo:     "info",
}

func registerWeeklyTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "weekly_actions",
		Description: "今週のアクション（要件 A-08）。体重トレンドから TDEE を逆算し、" +
			"推奨摂取と PFC、停滞・回復不足・記録漏れの検知を統合して、優先順位を付けた" +
			"「来週変えること」を返す。**上から順に対処する。** 記録漏れが出ているときは、" +
			"摂取やボリュームを動かす前に記録を直す。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in weeklyActionsIn) (*mcp.CallToolResult, weeklyActionsOut, error) {
		out, err := weeklyActions(ctx, d, in)

		return nil, out, err
	})
}

func weeklyActions(ctx context.Context, d Deps, in weeklyActionsIn) (weeklyActionsOut, error) {
	asof, err := parseAsOf(in.AsOf)
	if err != nil {
		return weeklyActionsOut{}, err
	}
	if d.Plan == nil {
		return weeklyActionsOut{}, fmt.Errorf(
			"計画の設定が読めていない。PHYSIQUE_CONFIG に config.json のパスを設定する")
	}

	out := weeklyActionsOut{AsOf: asof.Format(time.DateOnly)}

	goal, ok := d.Plan.GoalAt(asof)
	if !ok {
		out.Note = "計画の期間外。目標ペースが決まらないので、停滞判定は維持期として扱う"
	}
	out.GoalKgPerWeek = &goal
	out.Phase = phaseName(*d.Plan, asof)

	// 30日分あれば HRV の基準まで取れる。前週の歩数のためにもう1週さかのぼる
	from := openapi_types.Date{Time: asof.AddDate(0, 0, -(analytics.BaselineWindowDays + analytics.RecentWindowDays))}
	points, err := d.Analysis.DailySeries(ctx, from, openapi_types.Date{Time: asof})
	if err != nil {
		return weeklyActionsOut{}, err
	}

	stat := analytics.WeeklyStats(points, asof)
	out.WeightKg, out.BodyfatPct, out.SlopeKgWeek = stat.WeightKg7dAvg, stat.BodyfatPct7dAvg, stat.WeightSlopeKgWeek

	e1rm, err := worstKeyExerciseSlope(ctx, d, asof)
	if err != nil {
		return weeklyActionsOut{}, err
	}

	stalls := analytics.DetectStalls(analytics.BuildStallInput(points, asof, goal, e1rm))
	actx := analytics.ActionContext{GoalKgPerWeek: goal}

	// TDEE が出せるときだけ摂取の話をする。**推定できないのに数字を出さない**
	if stat.MeanKcal21d != nil && stat.WeightSlopeKgWeek != nil && stat.KcalDays21d >= tdeeMinDays {
		tdee := analytics.EstimateTDEE(*stat.MeanKcal21d, *stat.WeightSlopeKgWeek)
		out.TDEEKcal = &tdee

		if stat.WeightKg7dAvg != nil {
			rec := analytics.RecommendedIntake(tdee, goal, *stat.WeightKg7dAvg)
			mt := analytics.MacroTargets(d.Plan.NutritionConfig(), *stat.WeightKg7dAvg,
				stat.BodyfatPct7dAvg, goal, rec.RecommendedKcal)

			out.IntakeKcal = &rec.RecommendedKcal
			out.ProteinG, out.FatG, out.CarbG = &mt.ProteinG, &mt.FatG, &mt.CarbG
			actx.IntakeFloorHit = rec.FloorHit
			actx.CarbBelowFloor, actx.CarbTargetG = mt.CarbBelowFloor, mt.CarbG
		}
	} else if out.Note == "" {
		out.Note = fmt.Sprintf("TDEE を推定できない（直近%d日で摂取の記録が%d日 / 必要%d日）。"+
			"摂取量の意思決定にはこれが要る", analytics.TrendWindowDays, stat.KcalDays21d, tdeeMinDays)
	}

	for _, a := range analytics.WeeklyActions(stalls, actx) {
		out.Actions = append(out.Actions, actionOut{
			Kind: string(a.Kind), Priority: priorityLabel[a.Priority], Text: a.Text,
		})
	}

	return out, nil
}

// tdeeMinDays は TDEE を出すのに必要な摂取の記録日数。
// 足りない状態で出すと、数日の記録漏れがそのまま「代謝が落ちた」に化ける。
const tdeeMinDays = 10

// worstKeyExerciseSlope は主要種目の中で一番悪い e1RM の傾きを返す。
//
// 平均ではなく最悪値を見る。1種目でも落ちていれば回復かボリュームの問題で、
// 他が伸びていても打ち消されてはいけない。
func worstKeyExerciseSlope(ctx context.Context, d Deps, asof time.Time) (*float64, error) {
	from := openapi_types.Date{Time: asof.AddDate(0, 0, -analytics.E1RMWindowDays)}
	to := openapi_types.Date{Time: asof}

	all, err := d.Exercises.List(ctx, repository.ExerciseFilter{})
	if err != nil {
		return nil, err
	}
	idByName := make(map[string]uuid.UUID, len(all))
	for _, e := range all {
		idByName[e.Name] = e.Id
	}

	var worst *float64
	for _, name := range keyExercises {
		id, ok := idByName[name]
		if !ok {
			continue
		}

		points, err := d.Analysis.ExerciseHistory(ctx, id, from, to)
		if err != nil {
			return nil, err
		}

		days := make([]float64, 0, len(points))
		values := make([]float64, 0, len(points))
		for _, p := range points {
			// ExerciseHistory は from を含むが、窓の規則は左端を含めない。
			// ここを揃えないと1点多く入り、傾きが変わる
			if !analytics.InWindow(p.Date.Time, asof, analytics.E1RMWindowDays) {
				continue
			}
			days = append(days, p.Date.Time.Sub(asof).Hours()/24)
			values = append(values, p.BestE1RM)
		}

		slope := analytics.SlopePerWeek(days, values)
		if slope != nil && (worst == nil || *slope < *worst) {
			worst = slope
		}
	}

	return worst, nil
}

func phaseName(p plan.Plan, asof time.Time) string {
	day := asof.Format(time.DateOnly)
	for _, ph := range p.Phases {
		if ph.From <= day && day <= ph.To {
			return ph.Name
		}
	}

	return ""
}

func parseAsOf(s string) (time.Time, error) {
	if s == "" {
		return timeutil.Now().Truncate(24 * time.Hour), nil
	}

	t, err := time.Parse(time.DateOnly, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("asOf は YYYY-MM-DD で指定する: %q", s)
	}

	return t, nil
}
