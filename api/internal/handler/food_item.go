package handler

import (
	"context"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// ListFoodItems は食品マスタを返す（要件 N-02）。
func (s *Server) ListFoodItems(ctx context.Context, req openapi.ListFoodItemsRequestObject) (openapi.ListFoodItemsResponseObject, error) {
	if s.foodItems == nil {
		return openapi.ListFoodItems200JSONResponse{Items: []openapi.FoodItem{}}, nil
	}

	q := ""
	if req.Params.Q != nil {
		q = strings.TrimSpace(*req.Params.Q)
	}

	items, err := s.foodItems.List(ctx, q)
	if err != nil {
		return nil, err
	}
	// **null を返さない。** クライアントが map する前提
	if items == nil {
		items = []openapi.FoodItem{}
	}

	return openapi.ListFoodItems200JSONResponse{Items: items}, nil
}

// CreateFoodItem は食品マスタに登録する（要件 N-02）。
func (s *Server) CreateFoodItem(ctx context.Context, req openapi.CreateFoodItemRequestObject) (openapi.CreateFoodItemResponseObject, error) {
	if req.Body == nil {
		return createFoodItemFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateFoodItem(*req.Body); msg != "" {
		return createFoodItemFailed(field, msg), nil
	}
	if s.foodItems == nil {
		return createFoodItemFailed("foodItems", "食品マスタを保存できない設定になっている"), nil
	}

	it, err := s.foodItems.Create(ctx, normalizeFoodItem(*req.Body))
	if err != nil {
		return nil, err
	}

	return openapi.CreateFoodItem201JSONResponse(it), nil
}

// UpdateFoodItem は食品マスタを直す。**構成はまるごと置き換わる。**
func (s *Server) UpdateFoodItem(ctx context.Context, req openapi.UpdateFoodItemRequestObject) (openapi.UpdateFoodItemResponseObject, error) {
	if req.Body == nil {
		return updateFoodItemFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateFoodItem(*req.Body); msg != "" {
		return updateFoodItemFailed(field, msg), nil
	}
	if s.foodItems == nil {
		return openapi.UpdateFoodItem404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食品が見つからない", "")),
		}, nil
	}

	it, err := s.foodItems.Update(ctx, req.FoodItemId, normalizeFoodItem(*req.Body))
	if repository.IsNotFound(err) {
		return openapi.UpdateFoodItem404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食品が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.UpdateFoodItem200JSONResponse(it), nil
}

// DeleteFoodItem は食品マスタから消す（論理削除）。
func (s *Server) DeleteFoodItem(ctx context.Context, req openapi.DeleteFoodItemRequestObject) (openapi.DeleteFoodItemResponseObject, error) {
	if s.foodItems == nil {
		return openapi.DeleteFoodItem204Response{}, nil
	}

	err := s.foodItems.Delete(ctx, req.FoodItemId)
	if repository.IsNotFound(err) {
		return openapi.DeleteFoodItem404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "食品が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteFoodItem204Response{}, nil
}

// validateFoodItem は入力を検証する。
//
// **DB の check より手前で弾いて理由を返す。** 制約違反をそのまま返すと
// 「なぜ保存できないか」が利用者に伝わらない。
func validateFoodItem(in openapi.FoodItemInput) (field, message string) {
	if strings.TrimSpace(in.Name) == "" {
		return "name", "名前が空"
	}
	if len([]rune(in.Name)) > 200 {
		return "name", "200文字以下にする"
	}
	if in.Qty != nil && len([]rune(*in.Qty)) > 50 {
		return "qty", "50文字以下にする"
	}

	for _, m := range []struct {
		field string
		value *float32
		max   float32
	}{
		{"proteinG", in.ProteinG, 1000},
		{"fatG", in.FatG, 1000},
		{"carbG", in.CarbG, 2000},
	} {
		if msg := checkFloat(m.value, 0, m.max, false); msg != "" {
			return m.field, msg
		}
	}

	if in.Components == nil {
		return "", ""
	}

	// **入力量は名前で引く。** 重複すると片方しか届かない
	seen := map[string]bool{}
	for _, c := range *in.Components {
		name := strings.TrimSpace(c.Name)
		if name == "" {
			return "components", "引数の名前が空。どの欄かが分からなくなる"
		}
		if seen[name] {
			return "components", "引数の名前が重複している: " + name
		}
		seen[name] = true

		// **0 では割れない**（foodmaster.Expand の前提）
		if c.BasisAmount <= 0 {
			return "components", name + ": 基準量は 0 より大きくする"
		}
		if c.DefaultAmount < 0 {
			return "components", name + ": 既定値は 0 以上にする"
		}
		for _, m := range []struct {
			field string
			value *float32
			max   float32
		}{
			{"proteinG", c.ProteinG, 1000},
			{"fatG", c.FatG, 1000},
			{"carbG", c.CarbG, 2000},
		} {
			if msg := checkFloat(m.value, 0, m.max, false); msg != "" {
				return "components", name + ": " + m.field + " " + msg
			}
		}
	}

	return "", ""
}

// normalizeFoodItem は前後の空白を落とす。
func normalizeFoodItem(in openapi.FoodItemInput) openapi.FoodItemInput {
	out := in
	out.Name = strings.TrimSpace(in.Name)
	out.Qty = trimmedOrNil(in.Qty)

	if in.Components != nil {
		cs := make([]openapi.FoodItemComponent, 0, len(*in.Components))
		for _, c := range *in.Components {
			c.Name = strings.TrimSpace(c.Name)
			cs = append(cs, c)
		}
		out.Components = &cs
	}

	return out
}

func createFoodItemFailed(field, message string) openapi.CreateFoodItem422ApplicationProblemPlusJSONResponse {
	return openapi.CreateFoodItem422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func updateFoodItemFailed(field, message string) openapi.UpdateFoodItem422ApplicationProblemPlusJSONResponse {
	return openapi.UpdateFoodItem422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

// MarkFoodItemUsed は使った回数を1つ増やす（要件 N-02）。
//
// **失敗しても呼ぶ側は握ってよい。** 並び順が変わらないだけで、
// 記録そのものは済んでいる。
func (s *Server) MarkFoodItemUsed(ctx context.Context, req openapi.MarkFoodItemUsedRequestObject) (openapi.MarkFoodItemUsedResponseObject, error) {
	if s.foodItems == nil {
		return openapi.MarkFoodItemUsed204Response{}, nil
	}
	if err := s.foodItems.MarkUsed(ctx, req.FoodItemId); err != nil {
		return nil, err
	}

	return openapi.MarkFoodItemUsed204Response{}, nil
}
