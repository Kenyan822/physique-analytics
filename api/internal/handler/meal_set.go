package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// maxMealSetItems は食事セット1つあたりの項目数の上限。
const maxMealSetItems = 30

// ListMealSets は食事セットの一覧を返す（要件 N-03）。
func (s *Server) ListMealSets(ctx context.Context, _ openapi.ListMealSetsRequestObject) (openapi.ListMealSetsResponseObject, error) {
	items, err := s.mealSets.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.MealSet{}
	}

	return openapi.ListMealSets200JSONResponse{Items: items}, nil
}

// CreateMealSet は食事セットを作る。
func (s *Server) CreateMealSet(ctx context.Context, req openapi.CreateMealSetRequestObject) (openapi.CreateMealSetResponseObject, error) {
	if req.Body == nil {
		return createMealSetFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateMealSetInput(*req.Body); msg != "" {
		return createMealSetFailed(field, msg), nil
	}

	set, err := s.mealSets.Create(ctx, *req.Body)
	if repository.IsConflict(err) {
		return openapi.CreateMealSet409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "同じ名前の食事セットがある", "名前が同じだと選ぶときに区別できない")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.CreateMealSet201JSONResponse(set), nil
}

// UpdateMealSet は食事セットを更新する。項目は全入れ替え。
func (s *Server) UpdateMealSet(ctx context.Context, req openapi.UpdateMealSetRequestObject) (openapi.UpdateMealSetResponseObject, error) {
	if req.Body == nil {
		return updateMealSetFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateMealSetInput(*req.Body); msg != "" {
		return updateMealSetFailed(field, msg), nil
	}

	set, err := s.mealSets.Update(ctx, req.MealSetId, *req.Body)
	if repository.IsNotFound(err) {
		return openapi.UpdateMealSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食事セットが見つからない", "")),
		}, nil
	}
	if repository.IsConflict(err) {
		return updateMealSetFailed("name", "同じ名前の食事セットがある"), nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.UpdateMealSet200JSONResponse(set), nil
}

// DeleteMealSet は食事セットを論理削除する。
func (s *Server) DeleteMealSet(ctx context.Context, req openapi.DeleteMealSetRequestObject) (openapi.DeleteMealSetResponseObject, error) {
	err := s.mealSets.SoftDelete(ctx, req.MealSetId)
	if repository.IsNotFound(err) {
		return openapi.DeleteMealSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食事セットが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteMealSet204Response{}, nil
}

// ApplyMealSet はセットをその日の記録に展開する（要件 N-03）。
func (s *Server) ApplyMealSet(ctx context.Context, req openapi.ApplyMealSetRequestObject) (openapi.ApplyMealSetResponseObject, error) {
	if req.Body == nil {
		return openapi.ApplyMealSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "リクエストボディが無い", "")),
		}, nil
	}

	items, err := s.mealSets.Apply(ctx, req.MealSetId, req.Body.Date, req.Body.Slot)
	if repository.IsNotFound(err) {
		return openapi.ApplyMealSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食事セットが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.Meal{}
	}

	return openapi.ApplyMealSet201JSONResponse{Items: items}, nil
}

// validateMealSetInput は DB の check 制約と同じ範囲をハンドラ側で見る。
// 範囲は api/migrations/000007 の check と合わせてある。
func validateMealSetInput(in openapi.MealSetInput) (field, message string) {
	if strings.TrimSpace(in.Name) == "" {
		return "name", "名前が空"
	}
	if len([]rune(in.Name)) > 100 {
		return "name", "100文字以下にする"
	}
	// **空のセットを作らせない。** 展開しても何も起きず、壊れて見える
	if len(in.Items) == 0 {
		return "items", "項目が1つも無い"
	}
	if len(in.Items) > maxMealSetItems {
		return "items", fmt.Sprintf("%d件以下にする", maxMealSetItems)
	}

	for i, it := range in.Items {
		if msg := validateMealSetItem(it); msg != "" {
			return fmt.Sprintf("items[%d]", i), msg
		}
	}

	return "", ""
}

func validateMealSetItem(it openapi.MealSetItem) string {
	if strings.TrimSpace(it.Name) == "" {
		return "名前が空"
	}
	if len([]rune(it.Name)) > 200 {
		return "200文字以下にする"
	}
	if it.Qty != nil && len([]rune(*it.Qty)) > 50 {
		return "量は50文字以下にする"
	}
	if msg := checkInt(it.Kcal, 0, 10000, false); msg != "" {
		return msg
	}

	macros := []struct {
		v  *float32
		hi float32
	}{{it.ProteinG, 1000}, {it.FatG, 1000}, {it.CarbG, 2000}}
	for _, m := range macros {
		if msg := checkFloat(m.v, 0, m.hi, false); msg != "" {
			return msg
		}
	}

	return ""
}

func createMealSetFailed(field, message string) openapi.CreateMealSet422ApplicationProblemPlusJSONResponse {
	return openapi.CreateMealSet422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func updateMealSetFailed(field, message string) openapi.UpdateMealSet422ApplicationProblemPlusJSONResponse {
	return openapi.UpdateMealSet422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
