package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// defaultCorrelationDays は相関を見る既定の期間。
// 仕様は「最低90日」なので、既定では1年ぶん見て判断材料を増やす。
const defaultCorrelationDays = 365

type correlationsIn struct {
	Days int `json:"days,omitempty" jsonschema:"さかのぼる日数。既定 365"`
}

type correlationOut struct {
	Label    string   `json:"label"`
	N        int      `json:"n"`
	R        *float64 `json:"r,omitempty"`
	Strength string   `json:"strength"`
	Enough   bool     `json:"enough"`
}

type correlationsOut struct {
	From  string           `json:"from"`
	To    string           `json:"to"`
	Items []correlationOut `json:"items"`
	Note  string           `json:"note,omitempty"`
}

func registerCorrelationTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "correlations",
		Description: "個人の反応を見る相関分析（要件 A-12）。睡眠 → 翌日のトン数、" +
			"HRV → 翌日のトン数、歩数 → 体重変化、炭水化物 → その日のトン数。" +
			"**サンプルが90日に満たないものは enough=false で返す。** " +
			"少ないサンプルの相関は偶然を拾うので、行動を変える根拠にしない。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in correlationsIn) (*mcp.CallToolResult, correlationsOut, error) {
		out, err := correlations(ctx, d, in)

		return nil, out, err
	})
}

func correlations(ctx context.Context, d Deps, in correlationsIn) (correlationsOut, error) {
	days := in.Days
	if days <= 0 {
		days = defaultCorrelationDays
	}

	to := timeutil.Now().Truncate(24 * time.Hour)
	from := to.AddDate(0, 0, -days)

	points, err := d.Analysis.DailySeries(ctx,
		openapi_types.Date{Time: from}, openapi_types.Date{Time: to})
	if err != nil {
		return correlationsOut{}, err
	}

	tonnage, err := d.Analysis.DailyTonnage(ctx,
		openapi_types.Date{Time: from}, openapi_types.Date{Time: to})
	if err != nil {
		return correlationsOut{}, err
	}

	out := correlationsOut{
		From:  from.Format(time.DateOnly),
		To:    to.Format(time.DateOnly),
		Items: make([]correlationOut, 0, 4),
	}

	// 日付を「基準日からの経過日数」に直す。記録の無い日は飛ぶので、
	// 添字ではなく日付で突き合わせる必要がある
	dayOf := func(t time.Time) float64 { return t.Sub(from).Hours() / 24 }

	tonDays := make([]float64, 0, len(tonnage))
	tonVals := make([]float64, 0, len(tonnage))
	for _, t := range tonnage {
		tonDays = append(tonDays, dayOf(t.Date.Time))
		tonVals = append(tonVals, t.TonnageKg)
	}

	pick := func(get func(analytics.DailyPoint) *float64) (ds, vs []float64) {
		ds = make([]float64, 0, len(points))
		vs = make([]float64, 0, len(points))
		for _, p := range points {
			if v := get(p); v != nil {
				ds = append(ds, dayOf(p.Date))
				vs = append(vs, *v)
			}
		}

		return ds, vs
	}

	// 睡眠・HRV は「翌日」のパフォーマンスに効く（docs/03-分析ロジック.md 分析6）
	lagged := []struct {
		label string
		get   func(analytics.DailyPoint) *float64
	}{
		{"睡眠時間 → 翌日のトン数", func(p analytics.DailyPoint) *float64 { return p.SleepH }},
		{"深睡眠 → 翌日のトン数", func(p analytics.DailyPoint) *float64 { return p.DeepSleepMin }},
		{"HRV → 翌日のトン数", func(p analytics.DailyPoint) *float64 { return p.HrvMs }},
	}
	for _, l := range lagged {
		ds, vs := pick(l.get)
		xs, ys := analytics.LagPairs(ds, vs, tonDays, tonVals, 1)
		out.Items = append(out.Items, toCorrelationOut(analytics.Correlate(l.label, xs, ys)))
	}

	// 炭水化物はその日のトン数に効く
	ds, vs := pick(func(p analytics.DailyPoint) *float64 { return p.CarbG })
	xs, ys := analytics.LagPairs(ds, vs, tonDays, tonVals, 0)
	out.Items = append(out.Items, toCorrelationOut(analytics.Correlate("炭水化物 → その日のトン数", xs, ys)))

	// 歩数 → 体重の変化（翌日との差）。NEAT の寄与を見る
	wDays, wVals := pick(func(p analytics.DailyPoint) *float64 { return p.WeightKg })
	sDays, sVals := pick(func(p analytics.DailyPoint) *float64 { return p.Steps })
	nextW, curW := analytics.LagPairs(wDays, wVals, wDays, wVals, 1)
	deltas := make([]float64, 0, len(nextW))
	deltaDays := make([]float64, 0, len(nextW))
	for i := range nextW {
		deltas = append(deltas, curW[i]-nextW[i])
		deltaDays = append(deltaDays, wDays[i])
	}
	sx, sy := analytics.LagPairs(sDays, sVals, deltaDays, deltas, 0)
	out.Items = append(out.Items, toCorrelationOut(analytics.Correlate("歩数 → 翌日までの体重変化", sx, sy)))

	if allShort(out.Items) {
		out.Note = fmt.Sprintf("すべてサンプルが%d日に満たない。少ないサンプルの相関は偶然を拾うので、"+
			"行動を変える根拠にしない", analytics.MinCorrelationSamples)
	}

	return out, nil
}

func toCorrelationOut(c analytics.Correlation) correlationOut {
	return correlationOut{
		Label: c.Label, N: c.N, R: c.R,
		Strength: string(c.Strength), Enough: c.Enough,
	}
}

func allShort(items []correlationOut) bool {
	for _, i := range items {
		if i.Enough {
			return false
		}
	}

	return true
}
