package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func lastPerformanceServer(e *stubExercises, w *stubWorkouts) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, e, w, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}))
}

func TestGetLastPerformance_推定1RMを添えて返す(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	date := jstDateHandler("2026-09-12")
	sets := []openapi.WorkoutSet{
		// ウォームアップ相当。e1RM は低い
		{SetNo: 1, WeightKg: 60, Reps: 10, Rir: ptrInt(2)},
		// メインセット。ここが最大になる
		{SetNo: 2, WeightKg: 80, Reps: 8, Rir: ptrInt(2)},
	}
	w := &stubWorkouts{last: repository.LastPerformanceResult{Date: &date, Sets: sets}}

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{}, w).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+id.String()+"/last-performance", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var body openapi.GetLastPerformance200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.EstimatedOneRm == nil {
		t.Fatal("estimatedOneRm が無い")
	}
	// E1RM(80, 8, 2) = 106.67 が最大
	if diff := *body.EstimatedOneRm - 106.67; diff > 0.01 || diff < -0.01 {
		t.Errorf("estimatedOneRm = %v, want 106.67（最大のセット）", *body.EstimatedOneRm)
	}
}

// RIR が無いセットしかなければ推定1RMは出さない
func TestGetLastPerformance_RIRが無ければ推定1RMはnil(t *testing.T) {
	t.Parallel()

	date := jstDateHandler("2026-09-12")
	w := &stubWorkouts{last: repository.LastPerformanceResult{
		Date: &date,
		Sets: []openapi.WorkoutSet{{SetNo: 1, WeightKg: 80, Reps: 8}},
	}}

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{}, w).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String()+"/last-performance", nil))

	var body openapi.GetLastPerformance200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.EstimatedOneRm != nil {
		t.Errorf("estimatedOneRm = %v, want nil", *body.EstimatedOneRm)
	}
}

// 高レップ（限界12超）は Epley が過大評価するので対象外
func TestGetLastPerformance_高レップは推定1RMから除外される(t *testing.T) {
	t.Parallel()

	date := jstDateHandler("2026-09-12")
	w := &stubWorkouts{last: repository.LastPerformanceResult{
		Date: &date,
		Sets: []openapi.WorkoutSet{
			{SetNo: 1, WeightKg: 10, Reps: 20, Rir: ptrInt(0)}, // サイドレイズ相当
			{SetNo: 2, WeightKg: 80, Reps: 8, Rir: ptrInt(2)},
		},
	}}

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{}, w).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String()+"/last-performance", nil))

	var body openapi.GetLastPerformance200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.EstimatedOneRm == nil {
		t.Fatal("estimatedOneRm が無い")
	}
	if diff := *body.EstimatedOneRm - 106.67; diff > 0.01 || diff < -0.01 {
		t.Errorf("estimatedOneRm = %v, want 106.67（高レップを除外した値）", *body.EstimatedOneRm)
	}
}

// 前回値が無いのと種目が無いのは別。前者は 200 + date: null
func TestGetLastPerformance_未実施でも200(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{}, &stubWorkouts{}).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String()+"/last-performance", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var body openapi.GetLastPerformance200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.Date != nil {
		t.Errorf("date = %v, want null", body.Date)
	}
}

func TestGetLastPerformance_種目が無ければ404(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{getErr: repository.ErrNotFound}, &stubWorkouts{}).
		ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String()+"/last-performance", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

func ptrInt(v int) *int { return &v }
