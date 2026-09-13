package handler

import (
	"context"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// ListContests は大会の一覧を返す（要件 P-04）。
func (s *Server) ListContests(ctx context.Context, _ openapi.ListContestsRequestObject) (openapi.ListContestsResponseObject, error) {
	items, err := s.contests.List(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.Contest{}
	}

	return openapi.ListContests200JSONResponse{Items: items}, nil
}

// CreateContest は大会を登録する。
func (s *Server) CreateContest(ctx context.Context, req openapi.CreateContestRequestObject) (openapi.CreateContestResponseObject, error) {
	if req.Body == nil {
		return createContestFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateContestInput(*req.Body); msg != "" {
		return createContestFailed(field, msg), nil
	}

	c, err := s.contests.Create(ctx, *req.Body)
	if err != nil {
		return nil, err
	}

	return openapi.CreateContest201JSONResponse(c), nil
}

// UpdateContest は大会を更新する。
func (s *Server) UpdateContest(ctx context.Context, req openapi.UpdateContestRequestObject) (openapi.UpdateContestResponseObject, error) {
	if req.Body == nil {
		return updateContestFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateContestInput(*req.Body); msg != "" {
		return updateContestFailed(field, msg), nil
	}

	c, err := s.contests.Update(ctx, req.ContestId, *req.Body)
	if repository.IsNotFound(err) {
		return openapi.UpdateContest404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "大会が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.UpdateContest200JSONResponse(c), nil
}

// DeleteContest は大会を論理削除する。
func (s *Server) DeleteContest(ctx context.Context, req openapi.DeleteContestRequestObject) (openapi.DeleteContestResponseObject, error) {
	err := s.contests.SoftDelete(ctx, req.ContestId)
	if repository.IsNotFound(err) {
		return openapi.DeleteContest404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "大会が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteContest204Response{}, nil
}

// validateContestInput は DB の check 制約と同じ範囲をハンドラ側で見る。
// 範囲は api/migrations/000008 の check と合わせてある。
func validateContestInput(in openapi.ContestInput) (field, message string) {
	if strings.TrimSpace(in.Category) == "" {
		return "category", "カテゴリが空"
	}
	if len([]rune(in.Category)) > 200 {
		return "category", "200文字以下にする"
	}
	if in.TargetBfPct <= 0 || in.TargetBfPct >= 50 {
		return "targetBfPct", "0 より大きく 50 未満にする"
	}
	if in.Goal != nil && len([]rune(*in.Goal)) > 200 {
		return "goal", "200文字以下にする"
	}

	return "", ""
}

func createContestFailed(field, message string) openapi.CreateContest422ApplicationProblemPlusJSONResponse {
	return openapi.CreateContest422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func updateContestFailed(field, message string) openapi.UpdateContest422ApplicationProblemPlusJSONResponse {
	return openapi.UpdateContest422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
