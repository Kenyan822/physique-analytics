package handler_test

import (
	"encoding/json"
	"math"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func lastPerformanceServer(e *stubExercises, w *stubWorkouts) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, e, w, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
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

func getLastPerformance(t *testing.T, w *stubWorkouts, query string) *httptest.ResponseRecorder {
	t.Helper()

	rec := httptest.NewRecorder()
	lastPerformanceServer(&stubExercises{}, w).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/exercises/"+uuid.New().String()+"/last-performance"+query, nil))

	return rec
}

// before は repository にそのまま渡る。省略は nil（最新。互換）
func TestGetLastPerformance_beforeを渡す(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  string // 空なら nil
	}{
		{"省略すると nil", "", ""},
		{"指定した日", "?before=2026-10-08", "2026-10-08"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			w := &stubWorkouts{}
			rec := getLastPerformance(t, w, tt.query)

			if rec.Code != http.StatusOK {
				t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
			}
			switch {
			case tt.want == "" && w.gotBefore != nil:
				t.Errorf("before = %v, want nil", w.gotBefore)
			case tt.want != "" && (w.gotBefore == nil || w.gotBefore.Format("2006-01-02") != tt.want):
				t.Errorf("before = %v, want %s", w.gotBefore, tt.want)
			}
		})
	}
}

// before より前が無ければ date: null。推定1RMも無い
func TestGetLastPerformance_beforeより前が無ければnull(t *testing.T) {
	t.Parallel()

	rec := getLastPerformance(t, &stubWorkouts{}, "?before=2026-01-01")

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	// 未実施と同じ形。date は null か省略（クライアントはどちらも「初回」と読む）
	if v := body["date"]; v != nil {
		t.Errorf("date = %v, want null", v)
	}
	if v := body["estimatedOneRm"]; v != nil {
		t.Errorf("estimatedOneRm = %v, want null", v)
	}
}

// 推定1RMは返したセッションのセットから計算する（before で選んだ日のもの）
func TestGetLastPerformance_推定1RMは返したセッションのもの(t *testing.T) {
	t.Parallel()

	date := jstDateHandler("2026-09-12")
	w := &stubWorkouts{last: repository.LastPerformanceResult{
		Date: &date,
		Sets: []openapi.WorkoutSet{{WeightKg: 75, Reps: 8, Rir: ptrInt(2)}},
	}}
	rec := getLastPerformance(t, w, "?before=2026-09-20")

	var body openapi.GetLastPerformance200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.EstimatedOneRm == nil || math.Abs(float64(*body.EstimatedOneRm)-100) > 0.01 {
		t.Errorf("estimatedOneRm = %v, want 100（返したセットから）", body.EstimatedOneRm)
	}
}

// 不正な日付は生成コードが弾く。他のクエリ不正と同じく 400（problem+json）
func TestGetLastPerformance_不正な日付は400(t *testing.T) {
	t.Parallel()

	for _, q := range []string{"?before=yesterday", "?before=2026-13-45"} {
		rec := getLastPerformance(t, &stubWorkouts{}, q)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400 (body=%s)", q, rec.Code, rec.Body.String())
		}
	}
}

func ptrInt(v int) *int { return &v }
