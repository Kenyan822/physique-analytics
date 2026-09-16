package handler

import (
	"context"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// defaultSuggestionLimit は候補の既定件数。
// 食べるものは100品目程度に収束するので、上位20件あれば足りる。
const defaultSuggestionLimit = 20

// ListMeals は食事の一覧を返す。
func (s *Server) ListMeals(ctx context.Context, req openapi.ListMealsRequestObject) (openapi.ListMealsResponseObject, error) {
	items, err := s.meals.List(ctx, req.Params.From, req.Params.To)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.Meal{}
	}

	return openapi.ListMeals200JSONResponse{Items: items}, nil
}

// CreateMeal は食事を記録する（要件 N-01）。
func (s *Server) CreateMeal(ctx context.Context, req openapi.CreateMealRequestObject) (openapi.CreateMealResponseObject, error) {
	if req.Body == nil {
		return createMealFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateMealInput(*req.Body); msg != "" {
		return createMealFailed(field, msg), nil
	}

	m, err := s.meals.Create(ctx, toMealInput(*req.Body))
	if err != nil {
		return nil, err
	}

	return openapi.CreateMeal201JSONResponse(m), nil
}

// UpdateMeal は食事を更新する。
func (s *Server) UpdateMeal(ctx context.Context, req openapi.UpdateMealRequestObject) (openapi.UpdateMealResponseObject, error) {
	if req.Body == nil {
		return updateMealFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateMealInput(*req.Body); msg != "" {
		return updateMealFailed(field, msg), nil
	}

	m, err := s.meals.Update(ctx, req.MealId, toMealInput(*req.Body))
	if repository.IsNotFound(err) {
		return openapi.UpdateMeal404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食事が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.UpdateMeal200JSONResponse(m), nil
}

// DeleteMeal は食事を論理削除する。
func (s *Server) DeleteMeal(ctx context.Context, req openapi.DeleteMealRequestObject) (openapi.DeleteMealResponseObject, error) {
	err := s.meals.SoftDelete(ctx, req.MealId)
	if repository.IsNotFound(err) {
		return openapi.DeleteMeal404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食事が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteMeal204Response{}, nil
}

// ListMealSuggestions は過去の記録から候補を返す（要件 N-02）。
func (s *Server) ListMealSuggestions(ctx context.Context, req openapi.ListMealSuggestionsRequestObject) (openapi.ListMealSuggestionsResponseObject, error) {
	query := ""
	if req.Params.Q != nil {
		query = *req.Params.Q
	}
	limit := defaultSuggestionLimit
	if req.Params.Limit != nil {
		limit = *req.Params.Limit
	}

	items, err := s.meals.Suggestions(ctx, query, limit)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.MealSuggestion{}
	}

	return openapi.ListMealSuggestions200JSONResponse{Items: items}, nil
}

// CopyMeals は別の日の食事を複製する（要件 N-04）。
func (s *Server) CopyMeals(ctx context.Context, req openapi.CopyMealsRequestObject) (openapi.CopyMealsResponseObject, error) {
	if req.Body == nil {
		return copyMealsBadRequest("リクエストボディが無い"), nil
	}
	// 同じ日に複製すると倍になるだけで、意図した操作ではありえない
	if req.Body.FromDate.Format("2006-01-02") == req.Body.ToDate.Format("2006-01-02") {
		return copyMealsBadRequest("複製元と複製先が同じ日付"), nil
	}
	if req.Body.Slot != nil {
		if msg := checkEnum(*req.Body.Slot); msg != "" {
			return copyMealsBadRequest("slot: " + msg), nil
		}
	}

	items, err := s.meals.Copy(ctx, req.Body.FromDate, req.Body.ToDate, req.Body.Slot)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.Meal{}
	}

	return openapi.CopyMeals201JSONResponse{Items: items}, nil
}

// validateMealInput は DB の check 制約と同じ範囲をハンドラ側で見る。
//
// DB に任せると 23514 が 500 になり、何が悪いか返らない。
// 範囲は api/migrations/000005 の check と合わせてある。
func validateMealInput(in openapi.MealInput) (field, message string) {
	if strings.TrimSpace(in.Name) == "" {
		return "name", "名前が空"
	}
	if in.Slot != nil {
		if msg := checkEnum(*in.Slot); msg != "" {
			return "slot", msg
		}
	}
	if in.Source != nil {
		if msg := checkEnum(*in.Source); msg != "" {
			return "source", msg
		}
	}
	if len([]rune(in.Name)) > 200 {
		return "name", "200文字以下にする"
	}
	if in.Qty != nil && len([]rune(*in.Qty)) > 50 {
		return "qty", "50文字以下にする"
	}
	if msg := checkInt(in.Kcal, 0, 10000, false); msg != "" {
		return "kcal", msg
	}

	macros := []struct {
		field string
		v     *float32
		hi    float32
	}{
		{"proteinG", in.ProteinG, 1000},
		{"fatG", in.FatG, 1000},
		{"carbG", in.CarbG, 2000},
	}
	for _, m := range macros {
		if msg := checkFloat(m.v, 0, m.hi, false); msg != "" {
			return m.field, msg
		}
	}

	return "", ""
}

func toMealInput(in openapi.MealInput) repository.MealInput {
	out := repository.MealInput{
		Date:     in.Date,
		Slot:     in.Slot,
		Name:     strings.TrimSpace(in.Name),
		Qty:      in.Qty,
		Kcal:     in.Kcal,
		ProteinG: in.ProteinG,
		FatG:     in.FatG,
		CarbG:    in.CarbG,
		Source:   in.Source,
	}
	if in.Id != nil {
		id := *in.Id
		out.ID = &id
	}

	return out
}

func createMealFailed(field, message string) openapi.CreateMeal422ApplicationProblemPlusJSONResponse {
	return openapi.CreateMeal422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func updateMealFailed(field, message string) openapi.UpdateMeal422ApplicationProblemPlusJSONResponse {
	return openapi.UpdateMeal422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func copyMealsBadRequest(detail string) openapi.CopyMeals400ApplicationProblemPlusJSONResponse {
	return openapi.CopyMeals400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, "リクエストが不正", detail)),
	}
}
