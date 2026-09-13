package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
)

// GetPlan は計画の設定を返す（要件 P-01 / P-05）。
func (s *Server) GetPlan(ctx context.Context, _ openapi.GetPlanRequestObject) (openapi.GetPlanResponseObject, error) {
	p, err := s.plan.Get(ctx)
	if err != nil {
		return nil, err
	}

	return openapi.GetPlan200JSONResponse(p), nil
}

// PutPlan は計画の設定をまるごと置き換える。
func (s *Server) PutPlan(ctx context.Context, req openapi.PutPlanRequestObject) (openapi.PutPlanResponseObject, error) {
	if req.Body == nil {
		return planValidationFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validatePlanInput(*req.Body); msg != "" {
		return planValidationFailed(field, msg), nil
	}

	p, err := s.plan.Put(ctx, *req.Body)
	if err != nil {
		return nil, err
	}

	return openapi.PutPlan200JSONResponse(p), nil
}

// validatePlanInput は DB の check 制約に加えて、**制約では書けない関係**を見る。
//
// フェーズの重なりがそれで、1つの行を見ても判定できない。
// 重なりを許すとその日の目標ペースが一意に決まらず、停滞判定の基準が変わる。
func validatePlanInput(in openapi.PlanInput) (field, message string) {
	if field, msg := validatePhases(in.Phases); msg != "" {
		return field, msg
	}
	if field, msg := validateNutritionSettings(in.Nutrition); msg != "" {
		return field, msg
	}

	for _, v := range in.VolumeRanges {
		if !v.MuscleGroup.Valid() {
			return "volumeRanges", fmt.Sprintf("未知の部位: %q", v.MuscleGroup)
		}
		if v.Mev > v.Mrv {
			return "volumeRanges", fmt.Sprintf("%s: MEV が MRV を超えている", v.MuscleGroup)
		}
	}

	return "", ""
}

func validatePhases(phases []openapi.PlanPhase) (field, message string) {
	for _, p := range phases {
		if strings.TrimSpace(p.Name) == "" {
			return "phases", "フェーズ名が空"
		}
		if p.EndsOn.Before(p.StartsOn.Time) {
			return "phases", fmt.Sprintf("%s: 終了日が開始日より前", p.Name)
		}
	}

	// 総当たりで重なりを見る。フェーズは3年で数十件なので問題にならない
	for i := range phases {
		for j := i + 1; j < len(phases); j++ {
			if overlaps(phases[i], phases[j]) {
				return "phases", fmt.Sprintf("期間が重なっている: %s と %s", phases[i].Name, phases[j].Name)
			}
		}
	}

	return "", ""
}

func overlaps(a, b openapi.PlanPhase) bool {
	return !a.EndsOn.Before(b.StartsOn.Time) && !b.EndsOn.Before(a.StartsOn.Time)
}

func validateNutritionSettings(n openapi.NutritionSettings) (field, message string) {
	macros := []struct {
		field string
		v     openapi.MacroRatio
	}{
		{"nutrition.cut", n.Cut},
		{"nutrition.deepCut", n.DeepCut},
		{"nutrition.bulk", n.Bulk},
	}
	for _, m := range macros {
		if m.v.ProteinGPerKg <= 0 || m.v.ProteinGPerKg > 5 {
			return m.field, "タンパク質は 0 より大きく 5 g/kg 以下にする"
		}
		if m.v.FatGPerKg <= 0 || m.v.FatGPerKg > 5 {
			return m.field, "脂質は 0 より大きく 5 g/kg 以下にする"
		}
	}

	if n.DeepCutBfThreshold <= 0 || n.DeepCutBfThreshold >= 50 {
		return "nutrition.deepCutBfThreshold", "0 より大きく 50 未満にする"
	}
	if n.CarbMinG < 0 || n.CarbMinG > 1000 {
		return "nutrition.carbMinG", "0 以上 1000 以下にする"
	}

	return "", ""
}

func planValidationFailed(field, message string) openapi.PutPlan422ApplicationProblemPlusJSONResponse {
	return openapi.PutPlan422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
