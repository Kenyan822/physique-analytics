package handler

import (
	"context"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// GetDay は1日ぶんの要約を返す。
//
// **判定は analytics.BuildDayMeal に任せる**（/v1/streaks と同じ定義）。
// ここでは元データの取得と、レスポンスの形にするだけ
func (s *Server) GetDay(ctx context.Context, req openapi.GetDayRequestObject) (openapi.GetDayResponseObject, error) {
	var rec repository.DayRecords
	if s.days != nil {
		var err error
		if rec, err = s.days.Records(ctx, req.Date.Time); err != nil {
			return nil, err
		}
	}

	date := req.Date.Format("2006-01-02")
	meal := analytics.BuildDayMeal(date, rec.Targets, rec.Consumed)

	out := openapi.DayDetail{
		Date: req.Date,
		Meals: openapi.DayMeals{
			Consumed: openapi.Macros{
				Kcal:     float32(rec.Kcal),
				ProteinG: float32(rec.Consumed.ProteinG),
				FatG:     float32(rec.Consumed.FatG),
				CarbG:    float32(rec.Consumed.CarbG),
			},
			GoalMet: meal.GoalMet,
		},
	}
	if t := meal.Target; t != nil {
		out.Meals.Target = &openapi.Macros{
			Kcal:     float32(analytics.KcalFromMacros(t.ProteinG, t.FatG, t.CarbG)),
			ProteinG: float32(t.ProteinG),
			FatG:     float32(t.FatG),
			CarbG:    float32(t.CarbG),
		}
	}
	if d := meal.Shortfall; d != nil {
		out.Meals.Shortfall = &openapi.MacroDiff{
			ProteinG: float32(d.ProteinG),
			FatG:     float32(d.FatG),
			CarbG:    float32(d.CarbG),
		}
	}

	if w := rec.Workout; w != nil {
		out.Workout = &openapi.DayWorkout{
			TemplateName: w.TemplateName,
			DayOrder:     w.DayOrder,
			Exercises:    make([]openapi.DayWorkoutExercise, 0, len(w.Exercises)),
		}
		for _, e := range w.Exercises {
			out.Workout.SetCount += e.SetCount
			out.Workout.Exercises = append(out.Workout.Exercises, openapi.DayWorkoutExercise{
				ExerciseName: e.Name, SetCount: e.SetCount, TopWeightKg: float32(e.TopWeightKg),
			})
		}
	}

	if b := rec.Body; b != nil {
		out.Body = &openapi.DayBody{WeightKg: toFloat32(b.WeightKg), BodyFatPct: toFloat32(b.BodyFatPct)}
	}

	return openapi.GetDay200JSONResponse(out), nil
}

func toFloat32(v *float64) *float32 {
	if v == nil {
		return nil
	}
	f := float32(*v)

	return &f
}
