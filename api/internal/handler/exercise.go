package handler

import (
	"context"
	"log/slog"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// ListExercises は種目の一覧を返す。
func (s *Server) ListExercises(ctx context.Context, req openapi.ListExercisesRequestObject) (openapi.ListExercisesResponseObject, error) {
	f := repository.ExerciseFilter{MuscleGroup: req.Params.MuscleGroup}
	if req.Params.IncludeDeleted != nil {
		f.IncludeDeleted = *req.Params.IncludeDeleted
	}

	items, err := s.exercises.List(ctx, f)
	if err != nil {
		return nil, err
	}

	return openapi.ListExercises200JSONResponse{Items: items}, nil
}

// GetExercise は種目を1件返す。
func (s *Server) GetExercise(ctx context.Context, req openapi.GetExerciseRequestObject) (openapi.GetExerciseResponseObject, error) {
	e, err := s.exercises.Get(ctx, req.ExerciseId)
	if repository.IsNotFound(err) {
		return openapi.GetExercise404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse{
				Type:   "about:blank",
				Title:  "種目が見つからない",
				Status: 404,
			},
		}, nil
	}
	if err != nil {
		slog.ErrorContext(ctx, "種目を取得できない", slog.Any("error", err))
		return nil, err
	}

	return openapi.GetExercise200JSONResponse(e), nil
}

// CreateExercise は種目を追加する。
func (s *Server) CreateExercise(ctx context.Context, req openapi.CreateExerciseRequestObject) (openapi.CreateExerciseResponseObject, error) {
	if req.Body == nil {
		return createExerciseBadRequest("リクエストボディが無い", ""), nil
	}
	if msg := validateExerciseInput(*req.Body); msg != "" {
		return createExerciseBadRequest("入力が不正", msg), nil
	}

	e, err := s.exercises.Create(ctx, toExerciseInput(*req.Body))
	if repository.IsConflict(err) {
		// openapi.yaml は createExercise に 409 を定義していないので 400 で返す。
		// 同じ名前の種目が既にあることは、クライアント側で直せる入力ミス
		return createExerciseBadRequest("既に同じ名前の種目がある",
			"種目名 "+req.Body.Name+" は使われている。表記ゆれを避けるため重複は作れない"), nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.CreateExercise201JSONResponse(e), nil
}

// UpdateExercise は種目を更新する。
func (s *Server) UpdateExercise(ctx context.Context, req openapi.UpdateExerciseRequestObject) (openapi.UpdateExerciseResponseObject, error) {
	if req.Body == nil {
		return updateExerciseBadRequest("リクエストボディが無い", ""), nil
	}
	if msg := validateExerciseInput(*req.Body); msg != "" {
		return updateExerciseBadRequest("入力が不正", msg), nil
	}

	e, err := s.exercises.Update(ctx, req.ExerciseId, toExerciseInput(*req.Body))
	switch {
	case repository.IsNotFound(err):
		return openapi.UpdateExercise404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "種目が見つからない", "")),
		}, nil
	case repository.IsConflict(err):
		return openapi.UpdateExercise409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "既に同じ名前の種目がある", "種目名 "+req.Body.Name+" は使われている")),
		}, nil
	case err != nil:
		return nil, err
	}

	return openapi.UpdateExercise200JSONResponse(e), nil
}

// DeleteExercise は種目を論理削除する（ADR-0014）。
func (s *Server) DeleteExercise(ctx context.Context, req openapi.DeleteExerciseRequestObject) (openapi.DeleteExerciseResponseObject, error) {
	err := s.exercises.SoftDelete(ctx, req.ExerciseId)
	if repository.IsNotFound(err) {
		return openapi.DeleteExercise404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "種目が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteExercise204Response{}, nil
}

// validateExerciseInput は openapi.yaml のスキーマ制約のうち、生成コードが
// 検証してくれないものを見る。空なら問題なし。
func validateExerciseInput(in openapi.ExerciseInput) string {
	if in.Name == "" {
		return "name が空。1文字以上必要"
	}
	if len([]rune(in.Name)) > 100 {
		return "name が長すぎる（100文字まで）"
	}
	if !in.MuscleGroup.Valid() {
		return "muscleGroup が未知の値: " + string(in.MuscleGroup)
	}
	if in.DefaultRestSec != nil && *in.DefaultRestSec < 0 {
		return "defaultRestSec が負"
	}

	return ""
}

func toExerciseInput(in openapi.ExerciseInput) repository.ExerciseInput {
	return repository.ExerciseInput{
		ID:             in.Id,
		Name:           in.Name,
		MuscleGroup:    in.MuscleGroup,
		IsCompound:     in.IsCompound,
		DefaultRestSec: in.DefaultRestSec,
	}
}

func createExerciseBadRequest(title, detail string) openapi.CreateExercise400ApplicationProblemPlusJSONResponse {
	return openapi.CreateExercise400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}

func updateExerciseBadRequest(title, detail string) openapi.UpdateExercise400ApplicationProblemPlusJSONResponse {
	return openapi.UpdateExercise400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}
