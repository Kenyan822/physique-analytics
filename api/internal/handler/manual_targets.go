package handler

import (
	"context"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
)

// GetManualTargets は手で決めた摂取目標を返す（要件 N-05）。
func (s *Server) GetManualTargets(ctx context.Context, _ openapi.GetManualTargetsRequestObject) (openapi.GetManualTargetsResponseObject, error) {
	t, err := s.loadManualTargets(ctx)
	if err != nil {
		return nil, err
	}

	return openapi.GetManualTargets200JSONResponse{Targets: t}, nil
}

// PutManualTargets は手で決めた摂取目標を保存する（要件 N-05）。
func (s *Server) PutManualTargets(ctx context.Context, req openapi.PutManualTargetsRequestObject) (openapi.PutManualTargetsResponseObject, error) {
	if req.Body == nil {
		return manualTargetsFailed("body", "リクエストボディが無い"), nil
	}
	if s.manualTargets == nil {
		return manualTargetsFailed("targets", "手動の目標を保存できない設定になっている"), nil
	}

	in := *req.Body
	for _, c := range []struct {
		field string
		value float32
		max   float32
	}{
		{"proteinG", in.ProteinG, 1000},
		{"fatG", in.FatG, 1000},
		{"carbG", in.CarbG, 2000},
	} {
		if msg := checkFloat(&c.value, 0, c.max, false); msg != "" {
			return manualTargetsFailed(c.field, msg), nil
		}
	}

	saved, err := s.manualTargets.Put(ctx, in)
	if err != nil {
		return nil, err
	}

	return openapi.PutManualTargets200JSONResponse(saved), nil
}

// DeleteManualTargets は手で決めた摂取目標を消す。自動計算（A-02）に戻る。
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

// loadManualTargets は設定を読む。置き場所が無ければ nil。
func (s *Server) loadManualTargets(ctx context.Context) (*openapi.ManualTargets, error) {
	if s.manualTargets == nil {
		return nil, nil
	}

	return s.manualTargets.Get(ctx)
}

func manualTargetsFailed(field, message string) openapi.PutManualTargets422ApplicationProblemPlusJSONResponse {
	return openapi.PutManualTargets422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
