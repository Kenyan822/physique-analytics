package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// maxBloodTestItems は1回の検査で持てる項目数の上限。
const maxBloodTestItems = 200

// ListBloodTests は血液検査の一覧を返す（要件 B-08）。
func (s *Server) ListBloodTests(ctx context.Context, _ openapi.ListBloodTestsRequestObject) (openapi.ListBloodTestsResponseObject, error) {
	items, err := s.blood.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.BloodTest{}
	}

	return openapi.ListBloodTests200JSONResponse{Items: items}, nil
}

// CreateBloodTest は血液検査を登録する。
func (s *Server) CreateBloodTest(ctx context.Context, req openapi.CreateBloodTestRequestObject) (openapi.CreateBloodTestResponseObject, error) {
	if req.Body == nil {
		return createBloodTestFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateBloodTestInput(*req.Body); msg != "" {
		return createBloodTestFailed(field, msg), nil
	}

	t, err := s.blood.Create(ctx, *req.Body)
	if repository.IsConflict(err) {
		return openapi.CreateBloodTest409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "同じ日の血液検査がある", "直すなら既存のものを更新する")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.CreateBloodTest201JSONResponse(t), nil
}

// UpdateBloodTest は血液検査を更新する。項目は全入れ替え。
func (s *Server) UpdateBloodTest(ctx context.Context, req openapi.UpdateBloodTestRequestObject) (openapi.UpdateBloodTestResponseObject, error) {
	if req.Body == nil {
		return updateBloodTestFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateBloodTestInput(*req.Body); msg != "" {
		return updateBloodTestFailed(field, msg), nil
	}

	t, err := s.blood.Update(ctx, req.BloodTestId, *req.Body)
	if repository.IsNotFound(err) {
		return openapi.UpdateBloodTest404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "血液検査が見つからない", "")),
		}, nil
	}
	if repository.IsConflict(err) {
		return updateBloodTestFailed("date", "同じ日の血液検査がある"), nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.UpdateBloodTest200JSONResponse(t), nil
}

// DeleteBloodTest は血液検査を論理削除する。
func (s *Server) DeleteBloodTest(ctx context.Context, req openapi.DeleteBloodTestRequestObject) (openapi.DeleteBloodTestResponseObject, error) {
	err := s.blood.SoftDelete(ctx, req.BloodTestId)
	if repository.IsNotFound(err) {
		return openapi.DeleteBloodTest404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "血液検査が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteBloodTest204Response{}, nil
}

// validateBloodTestInput は DB の check 制約と同じ範囲をハンドラ側で見る。
// 範囲は api/migrations/000010 の check と合わせてある。
func validateBloodTestInput(in openapi.BloodTestInput) (field, message string) {
	if len(in.Items) == 0 {
		return "items", "検査項目が1つも無い"
	}
	if len(in.Items) > maxBloodTestItems {
		return "items", fmt.Sprintf("%d件以下にする", maxBloodTestItems)
	}
	if in.Clinic != nil && len([]rune(*in.Clinic)) > 100 {
		return "clinic", "100文字以下にする"
	}
	if in.Note != nil && len([]rune(*in.Note)) > 1000 {
		return "note", "1000文字以下にする"
	}

	for i, it := range in.Items {
		if msg := validateBloodTestItem(it); msg != "" {
			return fmt.Sprintf("items[%d]", i), msg
		}
	}

	return "", ""
}

func validateBloodTestItem(it openapi.BloodTestItem) string {
	if strings.TrimSpace(it.Name) == "" {
		return "項目名が空"
	}
	if len([]rune(it.Name)) > 100 {
		return "項目名は100文字以下にする"
	}
	if it.Unit != nil && len([]rune(*it.Unit)) > 30 {
		return "単位は30文字以下にする"
	}
	if it.TextValue != nil && len([]rune(*it.TextValue)) > 50 {
		return "文字の値は50文字以下にする"
	}
	// 下限が上限を超えていたら入力ミス
	if it.RefLow != nil && it.RefHigh != nil && *it.RefLow > *it.RefHigh {
		return "基準範囲の下限が上限を超えている"
	}

	return ""
}

func createBloodTestFailed(field, message string) openapi.CreateBloodTest422ApplicationProblemPlusJSONResponse {
	return openapi.CreateBloodTest422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func updateBloodTestFailed(field, message string) openapi.UpdateBloodTest422ApplicationProblemPlusJSONResponse {
	return openapi.UpdateBloodTest422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
