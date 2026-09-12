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
