package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

// stubSeries は日次記録の読み取りの差し替え。
type stubSeries struct {
	points []analytics.DailyPoint
}

func (s *stubSeries) DailySeries(context.Context, openapi_types.Date, openapi_types.Date) ([]analytics.DailyPoint, error) {
	return s.points, nil
}

// series は asof から n 日ぶんの記録を作る。
func series(n int, asof string, weight, kcal float64) []analytics.DailyPoint {
	d, err := parseDay(asof)
	if err != nil {
		panic(err)
	}

	out := make([]analytics.DailyPoint, 0, n)
	for i := range n {
		w := weight + 0.06*float64(i)
		k := kcal
		out = append(out, analytics.DailyPoint{
			Date: d.AddDate(0, 0, -i), WeightKg: &w, Kcal: &k,
		})
	}

	return out
}

func targetsServer(p *stubPlan, s *stubSeries, m *stubMeals) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, m, p, s))
}

func planWithPhase() openapi.Plan {
	return openapi.Plan{
		Phases: []openapi.PlanPhase{{
			Name:          "P1-A カット",
			StartsOn:      mustDate2("2026-10-01"),
			EndsOn:        mustDate2("2026-12-31"),
			GoalKgPerWeek: -0.5,
		}},
		Nutrition: openapi.NutritionSettings{
			Cut:                openapi.MacroRatio{ProteinGPerKg: 2.4, FatGPerKg: 0.85},
			DeepCut:            openapi.MacroRatio{ProteinGPerKg: 2.6, FatGPerKg: 0.85},
			Bulk:               openapi.MacroRatio{ProteinGPerKg: 2.2, FatGPerKg: 1.0},
			DeepCutBfThreshold: 13.0,
			CarbMinG:           200,
		},
	}
}

func TestGetDailyTargets_目標と残量を返す(t *testing.T) {
	t.Parallel()

	meals := &stubMeals{items: []openapi.Meal{
		{Name: "a", Kcal: ptr32i(600), ProteinG: ptr32(50), FatG: ptr32(20), CarbG: ptr32(45)},
		{Name: "b", Kcal: ptr32i(400), ProteinG: ptr32(30)},
	}}
	srv := targetsServer(&stubPlan{plan: planWithPhase()},
		&stubSeries{points: series(21, "2026-10-31", 75, 2100)}, meals)

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/targets/2026-10-31", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}

	if got.Consumed.Kcal != 1000 {
		t.Errorf("consumed.kcal = %v, want 1000", got.Consumed.Kcal)
	}
	if got.Consumed.ProteinG != 80 {
		t.Errorf("consumed.proteinG = %v, want 80", got.Consumed.ProteinG)
	}
	if got.Target == nil {
		t.Fatalf("target = nil, want 値。note = %v", got.Note)
	}
	if got.Remaining == nil {
		t.Fatal("remaining = nil")
	}
	// 残量 = 目標 − 実績
	if diff := got.Target.Kcal - got.Consumed.Kcal - got.Remaining.Kcal; diff > 0.01 || diff < -0.01 {
		t.Errorf("remaining が 目標 − 実績 になっていない: %v", got.Remaining.Kcal)
	}
	if got.TdeeKcal == nil {
		t.Error("tdeeKcal = nil")
	}
}

func TestGetDailyTargets_記録が足りなければ目標を出さない(t *testing.T) {
	t.Parallel()

	// **根拠の無い目標は判断を誤らせる。** 理由を note に入れる
	srv := targetsServer(&stubPlan{plan: planWithPhase()},
		&stubSeries{points: series(5, "2026-10-31", 75, 2100)}, &stubMeals{})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/targets/2026-10-31", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got.Target != nil {
		t.Errorf("target = %+v, want nil", got.Target)
	}
	if got.Note == nil || *got.Note == "" {
		t.Error("note が空。なぜ出せないかが分からない")
	}
	// 実績側は記録が無くても 0 で返す
	if got.Consumed.Kcal != 0 {
		t.Errorf("consumed.kcal = %v, want 0", got.Consumed.Kcal)
	}
}

func TestGetDailyTargets_フェーズが無ければ422(t *testing.T) {
	t.Parallel()

	srv := targetsServer(&stubPlan{plan: openapi.Plan{}}, &stubSeries{}, &stubMeals{})

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/targets/2026-10-31", nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
	// 直し方が返ること
	if !containsAll(rec.Body.String(), "フェーズ", "設定") {
		t.Errorf("body = %s, want 直し方を含む", rec.Body)
	}
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !jsonContains(s, sub) {
			return false
		}
	}

	return true
}

func jsonContains(s, sub string) bool {
	// JSON は日本語を \uXXXX でエスケープしないので単純な部分一致でよい
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}

		return false
	})()
}

func mustDate2(s string) openapi_types.Date {
	d, err := parseDay(s)
	if err != nil {
		panic(err)
	}

	return openapi_types.Date{Time: d}
}

func ptr32(v float32) *float32 { return &v }
func ptr32i(v int) *int        { return &v }

func parseDay(s string) (time.Time, error) {
	return time.Parse(time.DateOnly, s)
}
