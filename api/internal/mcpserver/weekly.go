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

type taperOut struct {
	Date           string   `json:"date"`
	NavyBodyfatPct *float64 `json:"navyBodyfatPct,omitempty"`
	ShoulderWaist  *float64 `json:"shoulderWaistRatio,omitempty"`
	VTaper         *bool    `json:"vTaper,omitempty"`
}

type deviationOut struct {
	Month      string  `json:"month"`
	Phase      string  `json:"phase"`
	LbmKg      float64 `json:"lbmKg"`
	BodyfatPct float64 `json:"bodyfatPct"`
	WeightKg   float64 `json:"weightKg"`
	Ffmi       float64 `json:"ffmi"`
	LbmBehind  bool    `json:"lbmBehind"`
}

type weeklyActionsOut struct {
	AsOf          string        `json:"asOf"`
	Phase         string        `json:"phase,omitempty"`
	GoalKgPerWeek *float64      `json:"goalKgPerWeek,omitempty"`
	TDEEKcal      *float64      `json:"tdeeKcal,omitempty"`
	IntakeKcal    *float64      `json:"intakeKcal,omitempty"`
	ProteinG      *float64      `json:"proteinG,omitempty"`
	FatG          *float64      `json:"fatG,omitempty"`
	CarbG         *float64      `json:"carbG,omitempty"`
	WeightKg      *float64      `json:"weightKg7dAvg,omitempty"`
	BodyfatPct    *float64      `json:"bodyfatPct7dAvg,omitempty"`
	SlopeKgWeek   *float64      `json:"weightSlopeKgPerWeek,omitempty"`
	LbmKg         *float64      `json:"lbmKg,omitempty"`
	Ffmi          *float64      `json:"ffmi,omitempty"`
	Taper         *taperOut     `json:"measurement,omitempty"`
	Deviation     *deviationOut `json:"deviationFromPlan,omitempty"`
	Contest       *contestOut   `json:"contest,omitempty"`
	Actions       []actionOut   `json:"actions"`
	Note          string        `json:"note,omitempty"`
}

var priorityLabel = map[analytics.ActionPriority]string{
	analytics.PriorityBlocker:  "blocker",
	analytics.PriorityRecovery: "recovery",
	analytics.PriorityAdjust:   "adjust",
	analytics.PriorityInfo:     "info",
}

type weeklyReportIn struct {
	AsOf string `json:"asOf,omitempty" jsonschema:"基準日 YYYY-MM-DD（JST）。省略時は今日"`
}

type weeklyReportOut struct {
	AsOf     string `json:"asOf"`
	Markdown string `json:"markdown"`
}

func registerWeeklyTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "weekly_report",
		Description: "週次レポートを Markdown で返す（要件 A-13）。体重トレンド・大会カウントダウン・" +
			"当月目標との進捗・推定TDEEと推奨摂取・周囲長・今週のアクションを1つの文書にまとめる。" +
			"**そのまま読ませる用。** 個々の数値を扱うなら weekly_actions を使う。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in weeklyReportIn) (*mcp.CallToolResult, weeklyReportOut, error) {
		out, err := weeklyReport(ctx, d, in)

		return nil, out, err
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "weekly_actions",
		Description: "今週のアクション（要件 A-08）。体重トレンドから TDEE を逆算し、" +
			"推奨摂取と PFC、体組成（LBM / 正規化FFMI）、月次目標との乖離、" +
			"停滞・回復不足・記録漏れの検知を統合して、優先順位を付けた" +
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

	sum, e1rm, err := buildSummary(ctx, d, asof)
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
	if c := sum.Composition; c != nil {
		out.LbmKg, out.Ffmi = &c.LbmKg, &c.Ffmi
	}
	if tp := sum.Taper; tp != nil {
		to := &taperOut{Date: tp.Date.Format(time.DateOnly), NavyBodyfatPct: tp.NavyBodyfatPct}
		if sw := tp.ShoulderWaist; sw != nil {
			to.ShoulderWaist, to.VTaper = &sw.Ratio, &sw.VTaper
		}
		out.Taper = to
	}
	if dv := sum.Deviation; dv != nil {
		out.Deviation = &deviationOut{
			Month: dv.Month, Phase: dv.Phase, LbmKg: dv.LbmKg,
			BodyfatPct: dv.BodyfatPct, WeightKg: dv.WeightKg, Ffmi: dv.Ffmi,
			LbmBehind: dv.LbmBehind,
		}
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

func weeklyReport(ctx context.Context, d Deps, in weeklyReportIn) (weeklyReportOut, error) {
	asof, err := parseAsOf(in.AsOf)
	if err != nil {
		return weeklyReportOut{}, err
	}

	sum, e1rm, err := buildSummary(ctx, d, asof)
	if err != nil {
		return weeklyReportOut{}, err
	}

	stalls := analytics.DetectStalls(analytics.BuildStallInput(sum.Points, asof, sum.GoalKgPerWeek, e1rm))

	return weeklyReportOut{
		AsOf:     asof.Format(time.DateOnly),
		Markdown: weekly.Markdown(sum, analytics.WeeklyActions(stalls, sum.ActionContext())),
	}, nil
}

// buildSummary は集計と e1RM の傾きをまとめて作る。
// weekly_actions と weekly_report が同じ入力から出ることを保証する。
func buildSummary(ctx context.Context, d Deps, asof time.Time) (weekly.Summary, *float64, error) {
	if d.Plan == nil {
		return weekly.Summary{}, nil, errors.New("計画の設定を読む口が無い（MCP の組み立てを確認する）")
	}

	deps := weekly.Deps{Plan: d.Plan, Series: d.Analysis}
	// **nil のポインタを interface に入れない。** 入れると interface 自体は
	// 非 nil になり、weekly 側の nil チェックをすり抜けて panic する
	if d.Contests != nil {
		deps.Contests = d.Contests
	}
	if d.Body != nil {
		deps.Measurements = d.Body
	}

	sum, err := weekly.Build(ctx, deps, asof)
	if err != nil {
		return weekly.Summary{}, nil, err
	}

	e1rm, err := worstKeyExerciseSlope(ctx, d, asof)
	if err != nil {
		return weekly.Summary{}, nil, err
	}

	return sum, e1rm, nil
}
