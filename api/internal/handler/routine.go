package handler

import (
	"context"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/routine"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// GetTodayRoutine は今日やる想定の種目を返す（要件 T-01 / #232）。
//
// **ルーティンが無くても 200 を返す。** 記録自体はできるので、
// 404 にすると画面が使えなくなる。
func (s *Server) GetTodayRoutine(ctx context.Context, req openapi.GetTodayRoutineRequestObject) (openapi.GetTodayRoutineResponseObject, error) {
	date := timeutil.Now().Format("2006-01-02")
	if req.Params.Date != nil {
		date = req.Params.Date.Format("2006-01-02")
	}
	out := openapi.TodayRoutine{Date: mustDate(date)}

	if s.routines == nil {
		return openapi.GetTodayRoutine200JSONResponse(out), nil
	}

	active, err := s.routines.Active(ctx)
	if err != nil {
		return nil, err
	}
	if active == nil {
		return openapi.GetTodayRoutine200JSONResponse(out), nil
	}
	out.RoutineName = &active.Name

	days, err := s.routines.Days(ctx, active.ID)
	if err != nil {
		return nil, err
	}
	if len(days) == 0 {
		return openapi.GetTodayRoutine200JSONResponse(out), nil
	}

	// **今日の途中では切り替えない。** 今日すでに記録があればその Day のまま
	last, err := s.routines.TodaySession(ctx, date)
	if err != nil {
		return nil, err
	}
	if last == nil {
		if last, err = s.routines.LastWithSets(ctx, date); err != nil {
			return nil, err
		}
	}
	if day := routine.Today(toRoutineDays(days), toRoutineSession(last), date); day != nil {
		order := day.Order
		out.TodayOrder = &order
	}

	// **前回値と今日の記録はまとめて引く。** Day ごとに叩くと往復が増える
	var ids []uuid.UUID
	for _, d := range days {
		for _, it := range d.Items {
			ids = append(ids, it.ExerciseID)
		}
	}
	lasts, err := s.routines.LastForExercises(ctx, ids)
	if err != nil {
		return nil, err
	}
	done, err := s.routines.DoneToday(ctx, date)
	if err != nil {
		return nil, err
	}

	all := make([]openapi.RoutineDay, 0, len(days))
	for _, d := range days {
		all = append(all, *toAPIDay(d, lasts, done))
	}
	out.Days = &all

	return openapi.GetTodayRoutine200JSONResponse(out), nil
}

func toRoutineDays(days []repository.RoutineDayRow) []routine.Day {
	out := make([]routine.Day, 0, len(days))
	for _, d := range days {
		out = append(out, routine.Day{Order: d.Order, TemplateID: d.TemplateID})
	}

	return out
}

func toRoutineSession(s *repository.SessionRow) *routine.Session {
	if s == nil {
		return nil
	}

	return &routine.Session{Date: s.Date, TemplateID: s.TemplateID}
}

func toAPIDay(
	d repository.RoutineDayRow,
	lasts map[uuid.UUID]repository.LastRow,
	done map[uuid.UUID]bool,
) *openapi.RoutineDay {
	items := make([]openapi.RoutineDayItem, 0, len(d.Items))
	for _, it := range d.Items {
		doneToday := done[it.ExerciseID]
		item := openapi.RoutineDayItem{
			ExerciseId:    it.ExerciseID,
			ExerciseName:  it.ExerciseName,
			MuscleGroup:   openapi.MuscleGroup(it.MuscleGroup),
			Order:         it.Order,
			TargetSets:    it.TargetSets,
			TargetRepsMin: it.TargetRepsMin,
			TargetRepsMax: it.TargetRepsMax,
			TargetRir:     it.TargetRir,
			DoneToday:     &doneToday,
		}
		if l, ok := lasts[it.ExerciseID]; ok {
			date := mustDate(l.Date)
			w := float32(l.WeightKg)
			reps := l.Reps
			item.LastDate = &date
			item.LastWeightKg = &w
			item.LastReps = &reps
			item.LastRir = l.Rir
		}
		items = append(items, item)
	}

	return &openapi.RoutineDay{
		DayOrder:     d.Order,
		TemplateId:   d.TemplateID,
		TemplateName: d.TemplateName,
		Items:        items,
	}
}

// mustDate は "YYYY-MM-DD" を openapi の Date にする。
//
// **呼ぶ前に形は保証されている**（DB の date 型 / timeutil.Now）。
// 読めないときはゼロ値を返す —— ここで落とす理由が無い。
func mustDate(s string) openapi_types.Date {
	t, err := time.ParseInLocation("2006-01-02", s, timeutil.JST)
	if err != nil {
		return openapi_types.Date{}
	}

	return openapi_types.Date{Time: t}
}
