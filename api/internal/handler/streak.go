package handler

import (
	"context"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// maxStreakDays は1回で返す最大の日数。カレンダーは月単位で、年間でも366日
const maxStreakDays = 366

// GetStreaks は日別の達成フラグを返す。
//
// 判定は analytics.BuildStreak に任せる。**ここでは期間の検証と元データの取得だけ**
func (s *Server) GetStreaks(ctx context.Context, req openapi.GetStreaksRequestObject) (openapi.GetStreaksResponseObject, error) {
	from, to := req.Params.From.Time, req.Params.To.Time

	if to.Before(from) {
		return streaksFailed("to", "from 以降にする"), nil
	}
	// 両端を含むので、日数は差 + 1
	if int(to.Sub(from).Hours()/24)+1 > maxStreakDays {
		return streaksFailed("to", "from から366日以内にする"), nil
	}

	var in repository.StreakInputs
	if s.streaks != nil {
		var err error
		if in, err = s.streaks.Inputs(ctx, from, to); err != nil {
			return nil, err
		}
	}

	days := analytics.BuildStreak(from, to, in.Targets, in.Consumed, in.Trained)

	items := make([]openapi.StreakDay, 0, len(days))
	for i, d := range days {
		// BuildStreak は from から1日ずつ、欠けなく返す
		items = append(items, openapi.StreakDay{
			Date:        openapi_types.Date{Time: from.AddDate(0, 0, i)},
			MealGoalMet: d.MealGoalMet,
			Trained:     d.Trained,
		})
	}

	return openapi.GetStreaks200JSONResponse{Items: items}, nil
}

func streaksFailed(field, message string) openapi.GetStreaks422ApplicationProblemPlusJSONResponse {
	return openapi.GetStreaks422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
