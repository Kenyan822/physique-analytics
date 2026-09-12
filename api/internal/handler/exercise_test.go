package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// stubExercises は handler.ExerciseRepository の差し替え。
// ハンドラ層で確かめたいのは「絞り込み条件が正しく渡るか」と
// 「ErrNotFound が 404 + Problem になるか」の2点で、SQL は repository のテストで見る。
type stubExercises struct {
	items      []openapi.Exercise
	gotFilter  *repository.ExerciseFilter
	getErr     error
	getReturns openapi.Exercise
}

func (s *stubExercises) List(_ context.Context, f repository.ExerciseFilter) ([]openapi.Exercise, error) {
	s.gotFilter = &f
	return s.items, nil
}

func (s *stubExercises) Get(context.Context, uuid.UUID) (openapi.Exercise, error) {
	return s.getReturns, s.getErr
}

func TestListExercises_一覧を返す(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{items: []openapi.Exercise{
		{Id: uuid.New(), Name: "ベンチプレス", MuscleGroup: openapi.Chest},
	}}
	rec := httptest.NewRecorder()
	handler.NewRouter(handler.New(stubPinger{}, stub)).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var body struct {
		Items []openapi.Exercise `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if len(body.Items) != 1 || body.Items[0].Name != "ベンチプレス" {
		t.Errorf("items = %+v", body.Items)
	}
}

func TestListExercises_クエリパラメータが絞り込み条件に渡る(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{}
	rec := httptest.NewRecorder()
	handler.NewRouter(handler.New(stubPinger{}, stub)).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises?muscleGroup=%E8%83%B8&includeDeleted=true", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotFilter == nil {
		t.Fatal("List が呼ばれていない")
	}
	if stub.gotFilter.MuscleGroup == nil || *stub.gotFilter.MuscleGroup != openapi.Chest {
		t.Errorf("muscleGroup = %v, want %q", stub.gotFilter.MuscleGroup, openapi.Chest)
	}
	if !stub.gotFilter.IncludeDeleted {
		t.Error("includeDeleted が渡っていない")
	}
}

// 既定では論理削除済みを含めない（openapi.yaml の IncludeDeleted の default: false）
func TestListExercises_includeDeleted未指定ならfalse(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{}
	rec := httptest.NewRecorder()
	handler.NewRouter(handler.New(stubPinger{}, stub)).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil))

	if stub.gotFilter == nil {
		t.Fatal("List が呼ばれていない")
	}
	if stub.gotFilter.IncludeDeleted {
		t.Error("includeDeleted が既定で true になっている")
	}
}

func TestGetExercise_存在しなければ404とProblem(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{getErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	handler.NewRouter(handler.New(stubPinger{}, stub)).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}

	var problem openapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if problem.Status != http.StatusNotFound {
		t.Errorf("problem.status = %d, want 404", problem.Status)
	}
}

func TestGetExercise_UUIDでないIDは400(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	handler.NewRouter(handler.New(stubPinger{}, &stubExercises{})).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises/not-a-uuid", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}
