package handler

import (
	"context"
	"fmt"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// openapi.yaml のスキーマ制約。生成コードは数値の範囲を検証しないので手で見る。
const (
	maxWeightKg = 500
	maxReps     = 100
	maxRIR      = 10
	maxSetNo    = 1000
)

// ListWorkoutSessions はセッションの一覧を返す。
func (s *Server) ListWorkoutSessions(ctx context.Context, req openapi.ListWorkoutSessionsRequestObject) (openapi.ListWorkoutSessionsResponseObject, error) {
	f := repository.SessionFilter{From: req.Params.From, To: req.Params.To}
	if req.Params.Limit != nil {
		f.Limit = *req.Params.Limit
	}

	items, err := s.workouts.ListSessions(ctx, f)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.WorkoutSession{}
	}

	return openapi.ListWorkoutSessions200JSONResponse{Items: items}, nil
}

// CreateWorkoutSession はセッションを作る。セットを含めて一括登録できる。
//
// 同じ id での再送は 200 を返す（openapi.yaml）。オフラインで記録した内容を
// 後から送る経路では、送信の成否が分からないまま再送されるため。
func (s *Server) CreateWorkoutSession(ctx context.Context, req openapi.CreateWorkoutSessionRequestObject) (openapi.CreateWorkoutSessionResponseObject, error) {
	if req.Body == nil {
		return workoutBadRequest("リクエストボディが無い", ""), nil
	}
	if req.Body.Date.IsZero() {
		return workoutBadRequest("date が無い", "JST の日付を YYYY-MM-DD で指定する"), nil
	}

	in := repository.SessionInput{
		ID:         req.Body.Id,
		Date:       req.Body.Date,
		TemplateID: req.Body.TemplateId,
		Note:       req.Body.Note,
	}
	if req.Body.Sets != nil {
		for i, set := range *req.Body.Sets {
			if msg := validateSet(set); msg != "" {
				return workoutValidationFailed(fmt.Sprintf("sets[%d]", i), msg), nil
			}
			in.Sets = append(in.Sets, toSetInput(set))
		}
	}

	session, existed, err := s.workouts.CreateSessionIdempotent(ctx, in)
	if err != nil {
		return nil, err
	}
	if existed {
		return openapi.CreateWorkoutSession200JSONResponse(session), nil
	}

	return openapi.CreateWorkoutSession201JSONResponse(session), nil
}

// GetWorkoutSession はセッションをセット込みで返す。
func (s *Server) GetWorkoutSession(ctx context.Context, req openapi.GetWorkoutSessionRequestObject) (openapi.GetWorkoutSessionResponseObject, error) {
	session, err := s.workouts.GetSession(ctx, req.SessionId)
	if repository.IsNotFound(err) {
		return openapi.GetWorkoutSession404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "セッションが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.GetWorkoutSession200JSONResponse(session), nil
}

// UpdateWorkoutSession はセッションを更新する。
func (s *Server) UpdateWorkoutSession(ctx context.Context, req openapi.UpdateWorkoutSessionRequestObject) (openapi.UpdateWorkoutSessionResponseObject, error) {
	if req.Body == nil {
		return openapi.UpdateWorkoutSession404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "リクエストボディが無い", "")),
		}, nil
	}

	up := repository.SessionUpdate{
		Date:              req.Body.Date,
		TemplateID:        req.Body.TemplateId,
		Note:              req.Body.Note,
		ExpectedUpdatedAt: req.Body.UpdatedAt,
	}

	session, err := s.workouts.UpdateSession(ctx, req.SessionId, up)
	switch {
	case repository.IsConflict(err):
		return openapi.UpdateWorkoutSession409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "サーバ側の更新の方が新しい",
					"最新を取得してから再度更新する（ADR-0014）")),
		}, nil
	case repository.IsNotFound(err):
		return openapi.UpdateWorkoutSession404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "セッションが見つからない", "")),
		}, nil
	case err != nil:
		return nil, err
	}

	return openapi.UpdateWorkoutSession200JSONResponse(session), nil
}

// DeleteWorkoutSession はセッションと配下のセットを論理削除する。
func (s *Server) DeleteWorkoutSession(ctx context.Context, req openapi.DeleteWorkoutSessionRequestObject) (openapi.DeleteWorkoutSessionResponseObject, error) {
	err := s.workouts.DeleteSession(ctx, req.SessionId)
	if repository.IsNotFound(err) {
		return openapi.DeleteWorkoutSession404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "セッションが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteWorkoutSession204Response{}, nil
}

// CreateWorkoutSet はセッションにセットを追加する。
func (s *Server) CreateWorkoutSet(ctx context.Context, req openapi.CreateWorkoutSetRequestObject) (openapi.CreateWorkoutSetResponseObject, error) {
	if req.Body == nil {
		return setValidationFailed("body", "リクエストボディが無い"), nil
	}
	if msg := validateSet(*req.Body); msg != "" {
		return setValidationFailed("set", msg), nil
	}

	set, err := s.workouts.CreateSet(ctx, req.SessionId, toSetInput(*req.Body))
	if err != nil {
		// openapi.yaml は createWorkoutSet に 404 を定義していない。
		// 存在しないセッションへの追加は「送った内容が仕様に合わない」なので 422 に寄せる
		if repository.IsNotFound(err) || repository.IsConflict(err) {
			return setValidationFailed("sessionId", err.Error()), nil
		}
		return nil, err
	}

	return openapi.CreateWorkoutSet201JSONResponse(set), nil
}

// UpdateWorkoutSet はセットを更新する。
func (s *Server) UpdateWorkoutSet(ctx context.Context, req openapi.UpdateWorkoutSetRequestObject) (openapi.UpdateWorkoutSetResponseObject, error) {
	if req.Body == nil {
		return openapi.UpdateWorkoutSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "リクエストボディが無い", "")),
		}, nil
	}

	set, err := s.workouts.UpdateSet(ctx, req.SetId, toSetInput(*req.Body))
	switch {
	case repository.IsConflict(err):
		return openapi.UpdateWorkoutSet409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "セット番号が重複している", "")),
		}, nil
	case repository.IsNotFound(err):
		return openapi.UpdateWorkoutSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "セットが見つからない", "")),
		}, nil
	case err != nil:
		return nil, err
	}

	return openapi.UpdateWorkoutSet200JSONResponse(set), nil
}

// DeleteWorkoutSet はセットを論理削除する。
func (s *Server) DeleteWorkoutSet(ctx context.Context, req openapi.DeleteWorkoutSetRequestObject) (openapi.DeleteWorkoutSetResponseObject, error) {
	err := s.workouts.DeleteSet(ctx, req.SetId)
	if repository.IsNotFound(err) {
		return openapi.DeleteWorkoutSet404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "セットが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteWorkoutSet204Response{}, nil
}

// validateSet は openapi.yaml の数値制約を見る。空なら問題なし。
func validateSet(in openapi.WorkoutSetInput) string {
	switch {
	case in.SetNo < 1 || in.SetNo > maxSetNo:
		return fmt.Sprintf("setNo が範囲外: %d（1 以上）", in.SetNo)
	case in.WeightKg < 0 || in.WeightKg > maxWeightKg:
		return fmt.Sprintf("weightKg が範囲外: %v（0〜%d）", in.WeightKg, maxWeightKg)
	case in.Reps < 0 || in.Reps > maxReps:
		return fmt.Sprintf("reps が範囲外: %d（0〜%d）", in.Reps, maxReps)
	case in.Rir != nil && (*in.Rir < 0 || *in.Rir > maxRIR):
		return fmt.Sprintf("rir が範囲外: %d（0〜%d）", *in.Rir, maxRIR)
	}

	return ""
}

func toSetInput(in openapi.WorkoutSetInput) repository.SetInput {
	return repository.SetInput{
		ID:         in.Id,
		ExerciseID: in.ExerciseId,
		SetNo:      in.SetNo,
		WeightKg:   in.WeightKg,
		Reps:       in.Reps,
		RIR:        in.Rir,
	}
}

func workoutBadRequest(title, detail string) openapi.CreateWorkoutSession400ApplicationProblemPlusJSONResponse {
	return openapi.CreateWorkoutSession400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}

func workoutValidationFailed(field, message string) openapi.CreateWorkoutSession422ApplicationProblemPlusJSONResponse {
	p := problem(422, "入力が仕様に合わない", message)
	p.Errors = &[]struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}{{Field: field, Message: message}}

	return openapi.CreateWorkoutSession422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(p),
	}
}

func setValidationFailed(field, message string) openapi.CreateWorkoutSet422ApplicationProblemPlusJSONResponse {
	p := problem(422, "入力が仕様に合わない", message)
	p.Errors = &[]struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}{{Field: field, Message: message}}

	return openapi.CreateWorkoutSet422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(p),
	}
}
