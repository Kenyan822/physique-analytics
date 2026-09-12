package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubWorkouts struct {
	sessions []openapi.WorkoutSession
	session  openapi.WorkoutSession
	set      openapi.WorkoutSet
	last     repository.LastPerformanceResult

	gotFilter   *repository.SessionFilter
	gotInput    *repository.SessionInput
	gotUpdate   *repository.SessionUpdate
	gotID       uuid.UUID
	alreadyMade bool

	listErr   error
	createErr error
	getErr    error
	updateErr error
	deleteErr error
	setErr    error
}

func (s *stubWorkouts) ListSessions(_ context.Context, f repository.SessionFilter) ([]openapi.WorkoutSession, error) {
	s.gotFilter = &f
	return s.sessions, s.listErr
}

func (s *stubWorkouts) CreateSessionIdempotent(_ context.Context, in repository.SessionInput) (openapi.WorkoutSession, bool, error) {
	s.gotInput = &in
	return s.session, s.alreadyMade, s.createErr
}

func (s *stubWorkouts) GetSession(_ context.Context, id uuid.UUID) (openapi.WorkoutSession, error) {
	s.gotID = id
	return s.session, s.getErr
}

func (s *stubWorkouts) UpdateSession(_ context.Context, id uuid.UUID, up repository.SessionUpdate) (openapi.WorkoutSession, error) {
	s.gotID = id
	s.gotUpdate = &up
	return s.session, s.updateErr
}

func (s *stubWorkouts) DeleteSession(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func (s *stubWorkouts) CreateSet(_ context.Context, _ uuid.UUID, _ repository.SetInput) (openapi.WorkoutSet, error) {
	return s.set, s.setErr
}

func (s *stubWorkouts) UpdateSet(_ context.Context, id uuid.UUID, _ repository.SetInput) (openapi.WorkoutSet, error) {
	s.gotID = id
	return s.set, s.setErr
}

func (s *stubWorkouts) DeleteSet(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func workoutServer(w *stubWorkouts) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, w, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}))
}

func TestCreateWorkoutSession_新規は201(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{session: openapi.WorkoutSession{Id: uuid.New()}}
	rec := postJSON(t, workoutServer(stub), http.MethodPost, "/v1/workout-sessions", map[string]any{
		"date": "2026-09-12",
		"note": "胸の日",
		"sets": []map[string]any{
			{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 80, "reps": 8, "rir": 2},
		},
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotInput == nil {
		t.Fatal("CreateSessionIdempotent が呼ばれていない")
	}
	if len(stub.gotInput.Sets) != 1 || stub.gotInput.Sets[0].WeightKg != 80 {
		t.Errorf("sets が渡っていない: %+v", stub.gotInput.Sets)
	}
	// JST の日付として解釈される（ADR-0013）
	if stub.gotInput.Date.Format("2006-01-02") != "2026-09-12" {
		t.Errorf("date = %v", stub.gotInput.Date)
	}
}

// 同じ id での再送は 200（openapi.yaml が 201 と 200 を出し分けている）
func TestCreateWorkoutSession_再送は200(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{session: openapi.WorkoutSession{Id: uuid.New()}, alreadyMade: true}
	rec := postJSON(t, workoutServer(stub), http.MethodPost, "/v1/workout-sessions", map[string]any{
		"id": uuid.New().String(), "date": "2026-09-12",
	})

	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateWorkoutSession_日付が無ければ400(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, workoutServer(&stubWorkouts{}), http.MethodPost, "/v1/workout-sessions",
		map[string]any{"note": "日付なし"})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// openapi.yaml: weightKg は 0..500、reps は 0..100、rir は 0..10
func TestCreateWorkoutSession_範囲外の値は422(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		set  map[string]any
	}{
		{"重量が上限超え", map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 600, "reps": 8}},
		{"重量が負", map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": -1, "reps": 8}},
		{"レップが上限超え", map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 80, "reps": 200}},
		{"RIRが上限超え", map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 80, "reps": 8, "rir": 20}},
		{"setNoが0", map[string]any{"exerciseId": uuid.New().String(), "setNo": 0, "weightKg": 80, "reps": 8}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, workoutServer(&stubWorkouts{}), http.MethodPost, "/v1/workout-sessions",
				map[string]any{"date": "2026-09-12", "sets": []map[string]any{tc.set}})

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestListWorkoutSessions_日付の範囲が渡る(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{}
	rec := httptest.NewRecorder()
	workoutServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/workout-sessions?from=2026-09-01&to=2026-09-30&limit=10", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotFilter == nil {
		t.Fatal("ListSessions が呼ばれていない")
	}
	if stub.gotFilter.From == nil || stub.gotFilter.From.Format("2006-01-02") != "2026-09-01" {
		t.Errorf("from = %v", stub.gotFilter.From)
	}
	if stub.gotFilter.Limit != 10 {
		t.Errorf("limit = %d, want 10", stub.gotFilter.Limit)
	}
}

func TestGetWorkoutSession_存在しなければ404(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{getErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	workoutServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
		"/v1/workout-sessions/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// ADR-0014: サーバ側が新しければ 409
func TestUpdateWorkoutSession_競合したら409(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{updateErr: repository.ErrConflict}
	rec := postJSON(t, workoutServer(stub), http.MethodPatch,
		"/v1/workout-sessions/"+uuid.New().String(),
		map[string]any{"note": "古い", "updatedAt": time.Now().Format(time.RFC3339)})

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestUpdateWorkoutSession_updatedAtが競合検出に渡る(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{session: openapi.WorkoutSession{Id: uuid.New()}}
	ts := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	rec := postJSON(t, workoutServer(stub), http.MethodPatch,
		"/v1/workout-sessions/"+uuid.New().String(),
		map[string]any{"note": "更新", "updatedAt": ts.Format(time.RFC3339)})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotUpdate == nil || stub.gotUpdate.ExpectedUpdatedAt == nil {
		t.Fatal("updatedAt が渡っていない。競合検出が効かない")
	}
	if !stub.gotUpdate.ExpectedUpdatedAt.Equal(ts) {
		t.Errorf("updatedAt = %v, want %v", stub.gotUpdate.ExpectedUpdatedAt, ts)
	}
}

func TestDeleteWorkoutSession_204(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	workoutServer(&stubWorkouts{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/workout-sessions/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestCreateWorkoutSet_201(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{set: openapi.WorkoutSet{Id: uuid.New(), SetNo: 1}}
	rec := postJSON(t, workoutServer(stub), http.MethodPost,
		"/v1/workout-sessions/"+uuid.New().String()+"/sets",
		map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 100, "reps": 5})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}

	var got openapi.WorkoutSet
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if got.SetNo != 1 {
		t.Errorf("setNo = %d", got.SetNo)
	}
}

func TestUpdateWorkoutSet_存在しなければ404(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{setErr: repository.ErrNotFound}
	rec := postJSON(t, workoutServer(stub), http.MethodPatch,
		"/v1/workout-sets/"+uuid.New().String(),
		map[string]any{"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 100, "reps": 5})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteWorkoutSet_204(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	workoutServer(&stubWorkouts{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/workout-sets/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

func (s *stubWorkouts) LastPerformance(_ context.Context, _ uuid.UUID) (repository.LastPerformanceResult, error) {
	return s.last, s.listErr
}

func jstDateHandler(s string) openapi_types.Date {
	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		panic(err)
	}

	return openapi_types.Date{Time: t}
}
