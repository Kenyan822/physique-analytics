package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
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

type contestOut struct {
	HeldOn         string   `json:"heldOn"`
	Category       string   `json:"category"`
	TargetBfPct    float64  `json:"targetBfPct"`
	WeeksLeft      float64  `json:"weeksLeft"`
	StageWeightKg  *float64 `json:"stageWeightKg,omitempty"`
	NeedLossKg     *float64 `json:"needLossKg,omitempty"`
	PacePctPerWeek *float64 `json:"pacePctPerWeek,omitempty"`
	TooFast        bool     `json:"tooFast"`
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
	Contest       *contestOut `json:"contest,omitempty"`
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
		return weeklyActionsOut{}, errors.New("計画の設定を読む口が無い（MCP の組み立てを確認する）")
	}

	// 集計と目標の組み立ては API と共通（internal/weekly）。
	// 同じ計算を2か所に書くと、片方だけ直したときに値が食い違う
	deps := weekly.Deps{Plan: d.Plan, Series: d.Analysis}
	// **nil のポインタを interface に入れない。** 入れると interface 自体は
	// 非 nil になり、weekly 側の nil チェックをすり抜けて panic する
	if d.Contests != nil {
		deps.Contests = d.Contests
	}

	sum, err := weekly.Build(ctx, deps, asof)
	if err != nil {
		return weeklyActionsOut{}, err
	}

	out := weeklyActionsOut{
		AsOf:        sum.AsOf.Format(time.DateOnly),
		Phase:       sum.Phase,
		TDEEKcal:    sum.TDEEKcal,
		WeightKg:    sum.WeightKg7dAvg,
		BodyfatPct:  sum.BodyfatPct7dAvg,
		SlopeKgWeek: sum.WeightSlopeKgWeek,
		Note:        sum.Note,
	}
	goal := sum.GoalKgPerWeek
	out.GoalKgPerWeek = &goal
	if t := sum.Targets; t != nil {
		out.IntakeKcal, out.ProteinG, out.FatG, out.CarbG = &t.KcalTarget, &t.ProteinG, &t.FatG, &t.CarbG
	}
	if c := sum.Contest; c != nil {
		co := &contestOut{
			HeldOn:      c.Contest.HeldOn.Format(time.DateOnly),
			Category:    c.Contest.Category,
			TargetBfPct: float64(c.Contest.TargetBfPct),
			WeeksLeft:   c.WeeksLeft,
		}
		if t := c.Target; t != nil {
			co.StageWeightKg, co.NeedLossKg = &t.StageWeightKg, &t.NeedLossKg
			co.PacePctPerWeek, co.TooFast = &t.PacePctPerWeek, t.TooFast
		}
		out.Contest = co
	}

	e1rm, err := worstKeyExerciseSlope(ctx, d, asof)
	if err != nil {
		return weeklyActionsOut{}, err
	}

	stalls := analytics.DetectStalls(analytics.BuildStallInput(sum.Points, asof, goal, e1rm))
	for _, a := range analytics.WeeklyActions(stalls, sum.ActionContext()) {
		out.Actions = append(out.Actions, actionOut{
			Kind: string(a.Kind), Priority: priorityLabel[a.Priority], Text: a.Text,
		})
	}

	return out, nil
}

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
