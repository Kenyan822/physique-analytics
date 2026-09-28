package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubRoutines struct {
	active *repository.RoutineRow
	days   []repository.RoutineDayRow
	last   *repository.SessionRow
	today  *repository.SessionRow
	lasts  map[uuid.UUID]repository.LastRow
	done   map[uuid.UUID]bool
}

func (s *stubRoutines) Active(context.Context) (*repository.RoutineRow, error) {
	return s.active, nil
}

func (s *stubRoutines) Days(context.Context, uuid.UUID) ([]repository.RoutineDayRow, error) {
	return s.days, nil
}

func (s *stubRoutines) LastWithSets(context.Context, string) (*repository.SessionRow, error) {
	return s.last, nil
}

func (s *stubRoutines) TodaySession(context.Context, string) (*repository.SessionRow, error) {
	return s.today, nil
}

func (s *stubRoutines) LastForExercises(context.Context, []uuid.UUID) (map[uuid.UUID]repository.LastRow, error) {
	return s.lasts, nil
}

func (s *stubRoutines) DoneToday(context.Context, string) (map[uuid.UUID]bool, error) {
	return s.done, nil
}

func routineServer(r handler.RoutineRepository) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{},
		&stubEstimator{}, &stubPhotos{}, &stubBlobs{})

	return handler.NewRouter(srv.WithRoutines(r))
}

var (
	tplA = uuid.MustParse("22222222-0000-0000-0000-00000000000a")
	tplB = uuid.MustParse("22222222-0000-0000-0000-00000000000b")
	exA  = uuid.MustParse("33333333-0000-0000-0000-00000000000a")
)

func twoDays() []repository.RoutineDayRow {
	return []repository.RoutineDayRow{
		{Order: 1, TemplateID: tplA, TemplateName: "胸", Items: []repository.RoutineItemRow{
			{ExerciseID: exA, ExerciseName: "ベンチプレス", MuscleGroup: "胸", Order: 1, TargetSets: 5},
		}},
		{Order: 2, TemplateID: tplB, TemplateName: "脚", Items: []repository.RoutineItemRow{
			{ExerciseID: uuid.New(), ExerciseName: "スクワット", MuscleGroup: "大腿四頭", Order: 1, TargetSets: 4},
		}},
	}
}

func getToday(t *testing.T, h http.Handler) map[string]any {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/routines/today?date=2026-09-28", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var out map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v", err)
	}

	return out
}

// **ルーティンが無くても 200。** 404 にすると記録画面が使えなくなる
func TestGetTodayRoutine_未登録でも200(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{}))

	if got["day"] != nil {
		t.Errorf("day = %v, want null", got["day"])
	}
	if got["date"] != "2026-09-28" {
		t.Errorf("date = %v", got["date"])
	}
}

func TestGetTodayRoutine_記録が無ければ1日目(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{
		active: &repository.RoutineRow{ID: uuid.New(), Name: "6日サイクル"},
		days:   twoDays(),
	}))

	day := got["day"].(map[string]any)
	if day["templateName"] != "胸" {
		t.Errorf("templateName = %v, want 胸", day["templateName"])
	}
	if got["totalDays"].(float64) != 2 {
		t.Errorf("totalDays = %v, want 2", got["totalDays"])
	}
}

func TestGetTodayRoutine_前回の次に進む(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{
		active: &repository.RoutineRow{ID: uuid.New(), Name: "x"},
		days:   twoDays(),
		last:   &repository.SessionRow{Date: "2026-09-26", TemplateID: &tplA},
	}))

	day := got["day"].(map[string]any)
	if day["templateName"] != "脚" {
		t.Errorf("templateName = %v, want 脚", day["templateName"])
	}
}

// **今日の途中で切り替わらない。** 入力中に並びが変わると混乱する
func TestGetTodayRoutine_今日すでに記録があればその日のまま(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{
		active: &repository.RoutineRow{ID: uuid.New(), Name: "x"},
		days:   twoDays(),
		today:  &repository.SessionRow{Date: "2026-09-28", TemplateID: &tplB},
		last:   &repository.SessionRow{Date: "2026-09-26", TemplateID: &tplA},
	}))

	day := got["day"].(map[string]any)
	if day["templateName"] != "脚" {
		t.Errorf("templateName = %v, want 脚（今日の記録のまま）", day["templateName"])
	}
}

func TestGetTodayRoutine_前回値を添えて返す(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{
		active: &repository.RoutineRow{ID: uuid.New(), Name: "x"},
		days:   twoDays(),
		lasts:  map[uuid.UUID]repository.LastRow{exA: {Date: "2026-09-20", WeightKg: 80, Reps: 8}},
		done:   map[uuid.UUID]bool{exA: true},
	}))

	item := got["day"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if item["lastWeightKg"].(float64) != 80 {
		t.Errorf("lastWeightKg = %v, want 80", item["lastWeightKg"])
	}
	if item["lastDate"] != "2026-09-20" {
		t.Errorf("lastDate = %v", item["lastDate"])
	}
	if item["doneToday"] != true {
		t.Errorf("doneToday = %v, want true", item["doneToday"])
	}
	// **部位も返す。** カテゴリ別に出すのに要る
	if item["muscleGroup"] != "胸" {
		t.Errorf("muscleGroup = %v, want 胸", item["muscleGroup"])
	}
}

func TestGetTodayRoutine_前回値が無ければnull(t *testing.T) {
	t.Parallel()

	got := getToday(t, routineServer(&stubRoutines{
		active: &repository.RoutineRow{ID: uuid.New(), Name: "x"},
		days:   twoDays(),
	}))

	item := got["day"].(map[string]any)["items"].([]any)[0].(map[string]any)
	if v, ok := item["lastDate"]; ok && v != nil {
		t.Errorf("lastDate = %v, want null", v)
	}
}
