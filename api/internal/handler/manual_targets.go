package handler

import (
	"context"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// GetManualTargets は手で決めた摂取目標を返す（要件 N-05）。
func (s *Server) GetManualTargets(ctx context.Context, _ openapi.GetManualTargetsRequestObject) (openapi.GetManualTargetsResponseObject, error) {
	t, err := s.loadManualTargets(ctx, timeutil.Now())
	if err != nil {
		return nil, err
	}

	return openapi.GetManualTargets200JSONResponse{Targets: t}, nil
}

// PutManualTargets は手で決めた摂取目標を、今日（JST）からのものとして履歴に足す（要件 N-05）。
func (s *Server) PutManualTargets(ctx context.Context, req openapi.PutManualTargetsRequestObject) (openapi.PutManualTargetsResponseObject, error) {
	if req.Body == nil {
		return manualTargetsFailed("body", "リクエストボディが無い"), nil
	}
	if s.manualTargets == nil {
		return manualTargetsFailed("targets", "手動の目標を保存できない設定になっている"), nil
	}

	in := *req.Body
	if field, msg := validateMacros(in.ProteinG, in.FatG, in.CarbG); msg != "" {
		return manualTargetsFailed(field, msg), nil
	}

	saved, err := s.manualTargets.Put(ctx, in)
	if err != nil {
		return nil, err
	}

	return openapi.PutManualTargets200JSONResponse(saved), nil
}

// DeleteManualTargets は手で決めた摂取目標を、履歴ごと全部消す。自動計算（A-02）に戻る。
func (s *Server) DeleteManualTargets(ctx context.Context, _ openapi.DeleteManualTargetsRequestObject) (openapi.DeleteManualTargetsResponseObject, error) {
	// **設定が無くても 204。** DELETE は冪等
	if s.manualTargets == nil {
		return openapi.DeleteManualTargets204Response{}, nil
	}
	if err := s.manualTargets.Delete(ctx); err != nil {
		return nil, err
	}

	return openapi.DeleteManualTargets204Response{}, nil
}

// loadManualTargets は date に適用される目標を読む。置き場所が無ければ nil。
func (s *Server) loadManualTargets(ctx context.Context, date time.Time) (*openapi.ManualTargets, error) {
	if s.manualTargets == nil {
		return nil, nil
	}

	return s.manualTargets.On(ctx, date)
}

func manualTargetsFailed(field, message string) openapi.PutManualTargets422ApplicationProblemPlusJSONResponse {
	return openapi.PutManualTargets422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

// ListManualTargetEntries は手動目標の履歴を返す（開始日の新しい順）。
func (s *Server) ListManualTargetEntries(ctx context.Context, _ openapi.ListManualTargetEntriesRequestObject) (openapi.ListManualTargetEntriesResponseObject, error) {
	if s.manualTargets == nil {
		return openapi.ListManualTargetEntries200JSONResponse{Items: []openapi.ManualTargetEntry{}}, nil
	}

	items, err := s.manualTargets.List(ctx)
	if err != nil {
		return nil, err
	}
	// null ではなく []。クライアントが items.map で落ちない
	if items == nil {
		items = []openapi.ManualTargetEntry{}
	}

	return openapi.ListManualTargetEntries200JSONResponse{Items: items}, nil
}

// CreateManualTargetEntry は履歴に1件足す。未来の日付なら予約になる。
func (s *Server) CreateManualTargetEntry(ctx context.Context, req openapi.CreateManualTargetEntryRequestObject) (openapi.CreateManualTargetEntryResponseObject, error) {
	if req.Body == nil {
		return entryFailed("body", "リクエストボディが無い"), nil
	}
	if s.manualTargets == nil {
		return entryFailed("targets", "手動の目標を保存できない設定になっている"), nil
	}

	in := *req.Body
	if in.Name != nil {
		if n := len([]rune(*in.Name)); n < 1 || n > 100 {
			return entryFailed("name", "1〜100文字にする"), nil
		}
	}
	if field, msg := validateMacros(in.ProteinG, in.FatG, in.CarbG); msg != "" {
		return entryFailed(field, msg), nil
	}

	saved, err := s.manualTargets.Add(ctx, in)
	if err != nil {
		return nil, err
	}

	return openapi.CreateManualTargetEntry201JSONResponse(saved), nil
}

// DeleteManualTargetEntry は履歴の1件を消す。
func (s *Server) DeleteManualTargetEntry(ctx context.Context, req openapi.DeleteManualTargetEntryRequestObject) (openapi.DeleteManualTargetEntryResponseObject, error) {
	if s.manualTargets == nil {
		return notFoundEntry(), nil
	}

	err := s.manualTargets.DeleteEntry(ctx, req.EntryId)
	if repository.IsNotFound(err) {
		return notFoundEntry(), nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteManualTargetEntry204Response{}, nil
}

// validateMacros は PFC の範囲を見る。上限は openapi.yaml と DB の check に合わせる。
func validateMacros(proteinG, fatG, carbG float32) (field, message string) {
	for _, c := range []struct {
		field string
		value float32
		max   float32
	}{
		{"proteinG", proteinG, 1000},
		{"fatG", fatG, 1000},
		{"carbG", carbG, 2000},
	} {
		if msg := checkFloat(&c.value, 0, c.max, false); msg != "" {
			return c.field, msg
		}
	}

	return "", ""
}

func entryFailed(field, message string) openapi.CreateManualTargetEntry422ApplicationProblemPlusJSONResponse {
	return openapi.CreateManualTargetEntry422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func notFoundEntry() openapi.DeleteManualTargetEntry404ApplicationProblemPlusJSONResponse {
	return openapi.DeleteManualTargetEntry404ApplicationProblemPlusJSONResponse{
		NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
			problem(404, "目標の履歴が見つからない", "")),
	}
}
