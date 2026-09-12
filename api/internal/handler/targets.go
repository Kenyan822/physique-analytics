package handler

import (
	"context"
	"errors"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
)

// GetDailyTargets はその日の摂取目標と残量を返す（要件 N-05）。
//
// 計算は internal/weekly と共有している。MCP の週次レポート（A-08）と
// 同じ値を出すため、ここで計算し直さない。
func (s *Server) GetDailyTargets(ctx context.Context, req openapi.GetDailyTargetsRequestObject) (openapi.GetDailyTargetsResponseObject, error) {
	sum, err := weekly.Build(ctx, weekly.Deps{Plan: s.plan, Series: s.series}, req.Date.Time)
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
