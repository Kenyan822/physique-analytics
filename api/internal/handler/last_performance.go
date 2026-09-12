package handler

import (
	"context"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// GetLastPerformance は種目の前回実施内容を返す（要件 T-02）。
//
// 入力速度を決める最重要機能。次のセットのデフォルト値をここから埋めるため、
// 1リクエストで「前回いつ・何kg×何レップ・RIRいくつ」が揃う形にしている。
func (s *Server) GetLastPerformance(ctx context.Context, req openapi.GetLastPerformanceRequestObject) (openapi.GetLastPerformanceResponseObject, error) {
	// 存在しない種目は 404。前回値が無いだけの種目（Date が nil）とは区別する
	if _, err := s.exercises.Get(ctx, req.ExerciseId); err != nil {
		if repository.IsNotFound(err) {
			return openapi.GetLastPerformance404ApplicationProblemPlusJSONResponse{
				NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
					problem(404, "種目が見つからない", "")),
			}, nil
		}
		return nil, err
	}

	last, err := s.workouts.LastPerformance(ctx, req.ExerciseId)
	if err != nil {
		return nil, err
	}

	return openapi.GetLastPerformance200JSONResponse{
		ExerciseId:     req.ExerciseId,
		Date:           last.Date,
		Sets:           &last.Sets,
		EstimatedOneRm: bestE1RM(last.Sets),
	}, nil
}

// bestE1RM は前回のセットのうち最も高い推定1RMを返す。
//
// 平均ではなく最大を採る。ウォームアップを含むセット群の平均には意味がなく、
// 「前回どこまで挙げられたか」が知りたい値だから。
// RIR 未記録や高レップのセットは対象外（analytics.UsableForE1RM）。
func bestE1RM(sets []openapi.WorkoutSet) *float32 {
	var best float64
	for _, s := range sets {
		if !analytics.UsableForE1RM(s.Reps, s.Rir) {
			continue
		}
		if v := analytics.E1RM(float64(s.WeightKg), float64(s.Reps), float64(*s.Rir)); v > best {
			best = v
		}
	}
	if best == 0 {
		return nil
	}

	v := float32(best)

	return &v
}
