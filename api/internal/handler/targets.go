package handler

import (
	"context"
	"errors"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
)

// GetDailyTargets はその日の摂取目標と残量を返す（要件 N-05）。
//
// 計算は internal/weekly と共有している。MCP の週次レポート（A-08）と
// 同じ値を出すため、ここで計算し直さない。
func (s *Server) GetDailyTargets(ctx context.Context, req openapi.GetDailyTargetsRequestObject) (openapi.GetDailyTargetsResponseObject, error) {
	deps := weekly.Deps{Plan: s.plan, Series: s.series}
	// nil のポインタを interface に入れると非 nil になり、nil チェックをすり抜ける
	if s.contests != nil {
		deps.Contests = s.contests
	}
	if s.body != nil {
		deps.Measurements = s.body
	}

	sum, err := weekly.Build(ctx, deps, req.Date.Time)
	if errors.Is(err, weekly.ErrNoPhases) {
		return targetsValidationFailed("phases", err.Error()), nil
	}
	if err != nil {
		return nil, err
	}

	meals, err := s.meals.List(ctx, &req.Date, &req.Date)
	if err != nil {
		return nil, err
	}

	out := openapi.DailyTargets{
		Date:     req.Date,
		Consumed: sumMeals(meals),
	}
	if sum.Phase != "" {
		phase := sum.Phase
		out.Phase = &phase
	}
	goal := float32(sum.GoalKgPerWeek)
	out.GoalKgPerWeek = &goal
	if sum.TDEEKcal != nil {
		tdee := float32(*sum.TDEEKcal)
		out.TdeeKcal = &tdee
	}
	if sum.Note != "" {
		note := sum.Note
		out.Note = &note
	}

	if c := sum.Contest; c != nil {
		cd := openapi.ContestCountdown{
			HeldOn:      c.Contest.HeldOn,
			Category:    c.Contest.Category,
			TargetBfPct: c.Contest.TargetBfPct,
			WeeksLeft:   float32(c.WeeksLeft),
		}
		if t := c.Target; t != nil {
			stage, loss, pace := float32(t.StageWeightKg), float32(t.NeedLossKg), float32(t.PacePctPerWeek)
			cd.StageWeightKg, cd.NeedLossKg, cd.PacePctPerWeek = &stage, &loss, &pace
			cd.TooFast = &t.TooFast
		}
		out.Contest = &cd
	}

	if t := sum.Targets; t != nil {
		target := openapi.Macros{
			Kcal:     float32(t.KcalTarget),
			ProteinG: float32(t.ProteinG),
			FatG:     float32(t.FatG),
			CarbG:    float32(t.CarbG),
		}
		out.Target = &target
		remaining := openapi.Macros{
			Kcal:     target.Kcal - out.Consumed.Kcal,
			ProteinG: target.ProteinG - out.Consumed.ProteinG,
			FatG:     target.FatG - out.Consumed.FatG,
			CarbG:    target.CarbG - out.Consumed.CarbG,
		}
		out.Remaining = &remaining
		out.IntakeFloorHit = &t.IntakeFloorHit
		out.CarbBelowFloor = &t.CarbBelowFloor

		suggestions, err := s.suggestFoods(ctx, remaining)
		if err != nil {
			return nil, err
		}
		out.Suggestions = &suggestions
	}

	return openapi.GetDailyTargets200JSONResponse(out), nil
}

// sumMeals は実績を合計する。
//
// **未入力（null）は 0 として足す。** 0 を入れて辻褄を合わせるより、
// 合計が少なく出る方がよい（記録漏れの判定は分析側が持つ）。
func sumMeals(meals []openapi.Meal) openapi.Macros {
	var out openapi.Macros
	for _, m := range meals {
		if m.Kcal != nil {
			out.Kcal += float32(*m.Kcal)
		}
		if m.ProteinG != nil {
			out.ProteinG += *m.ProteinG
		}
		if m.FatG != nil {
			out.FatG += *m.FatG
		}
		if m.CarbG != nil {
			out.CarbG += *m.CarbG
		}
	}

	return out
}

func targetsValidationFailed(field, message string) openapi.GetDailyTargets422ApplicationProblemPlusJSONResponse {
	return openapi.GetDailyTargets422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

// maxFoodSuggestions は提案する件数。多く出しても選べない
const maxFoodSuggestions = 5

// suggestFoods は残量を埋める食品を履歴から提案する（要件 N-07）。
//
// **食品マスタは持たない**ので、候補は自分の記録履歴から来る。
// 食べたことのないものは提案しない。
func (s *Server) suggestFoods(ctx context.Context, remaining openapi.Macros) ([]openapi.FoodSuggestion, error) {
	// 候補は多めに取る。PFC の無い記録が混ざるため
	history, err := s.meals.Suggestions(ctx, "", maxFoodSuggestions*10)
	if err != nil {
		return nil, err
	}

	candidates := make([]analytics.FoodCandidate, 0, len(history))
	for _, h := range history {
		candidates = append(candidates, analytics.FoodCandidate{
			Name:     h.Name,
			Count:    h.Count,
			Kcal:     floatOf(h.Kcal),
			ProteinG: float32Of(h.ProteinG),
			FatG:     float32Of(h.FatG),
			CarbG:    float32Of(h.CarbG),
		})
	}

	rem := analytics.Remaining{
		Kcal:     float64(remaining.Kcal),
		ProteinG: float64(remaining.ProteinG),
		FatG:     float64(remaining.FatG),
		CarbG:    float64(remaining.CarbG),
	}

	out := make([]openapi.FoodSuggestion, 0, maxFoodSuggestions)
	for _, sg := range analytics.SuggestFoods(rem, candidates, maxFoodSuggestions) {
		out = append(out, openapi.FoodSuggestion{
			Name:            sg.Name,
			Fits:            sg.Fits,
			FillsProteinPct: float32(sg.FillsProteinPct),
		})
	}

	return out, nil
}

func floatOf(v *int) float64 {
	if v == nil {
		return 0
	}

	return float64(*v)
}

func float32Of(v *float32) float64 {
	if v == nil {
		return 0
	}

	return float64(*v)
}
