package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

type stubPlan struct {
	plan     openapi.Plan
	gotInput *openapi.PlanInput
	putErr   error
}

func (s *stubPlan) Get(context.Context) (openapi.Plan, error) { return s.plan, nil }

func (s *stubPlan) Put(_ context.Context, in openapi.PlanInput) (openapi.Plan, error) {
	s.gotInput = &in
	return s.plan, s.putErr
}

func planServer(p *stubPlan) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, p, &stubSeries{}, &stubMealSets{}))
}

func validPlan() map[string]any {
	return map[string]any{
		"heightCm": 175,
		"phases": []map[string]any{
			{"name": "P1-A", "startsOn": "2026-10-01", "endsOn": "2026-10-31", "goalKgPerWeek": -0.76},
		},
		"nutrition": map[string]any{
			"cut":                map[string]any{"proteinGPerKg": 2.4, "fatGPerKg": 0.85},
			"deepCut":            map[string]any{"proteinGPerKg": 2.6, "fatGPerKg": 0.85},
			"bulk":               map[string]any{"proteinGPerKg": 2.2, "fatGPerKg": 1.0},
			"deepCutBfThreshold": 13.0,
			"carbMinG":           200,
		},
		"volumeRanges": []map[string]any{{"muscleGroup": "胸", "mev": 10, "mrv": 20}},
	}
}

func TestGetPlan_設定を返す(t *testing.T) {
	t.Parallel()

	stub := &stubPlan{plan: openapi.Plan{
		Phases:       []openapi.PlanPhase{{Name: "P1-A"}},
		VolumeRanges: []openapi.VolumeRange{{MuscleGroup: openapi.Chest, Mev: 10, Mrv: 20}},
	}}
	rec := httptest.NewRecorder()
	planServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/plan", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var got openapi.Plan
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(got.Phases) != 1 {
		t.Errorf("Phases = %d 件, want 1", len(got.Phases))
	}
}

func TestPutPlan_入力を渡す(t *testing.T) {
	t.Parallel()

	stub := &stubPlan{}
	rec := postJSON(t, planServer(stub), http.MethodPut, "/v1/plan", validPlan())

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput == nil {
		t.Fatal("Put が呼ばれていない")
	}
	if len(stub.gotInput.Phases) != 1 {
		t.Errorf("Phases = %d 件, want 1", len(stub.gotInput.Phases))
	}
}

func TestPutPlan_検証(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"フェーズの期間が逆", func(m map[string]any) {
			m["phases"] = []map[string]any{
				{"name": "x", "startsOn": "2026-10-31", "endsOn": "2026-10-01", "goalKgPerWeek": 0},
			}
		}},
		{"フェーズ名が空", func(m map[string]any) {
			m["phases"] = []map[string]any{
				{"name": "  ", "startsOn": "2026-10-01", "endsOn": "2026-10-31", "goalKgPerWeek": 0},
			}
		}},
		{"MEV が MRV を超える", func(m map[string]any) {
			m["volumeRanges"] = []map[string]any{{"muscleGroup": "胸", "mev": 20, "mrv": 10}}
		}},
		{"未知の部位", func(m map[string]any) {
			m["volumeRanges"] = []map[string]any{{"muscleGroup": "存在しない部位", "mev": 10, "mrv": 20}}
		}},
		{"タンパク質の係数が0", func(m map[string]any) {
			n := m["nutrition"].(map[string]any)
			n["cut"] = map[string]any{"proteinGPerKg": 0, "fatGPerKg": 0.85}
		}},
		{"体脂肪率の閾値が範囲外", func(m map[string]any) {
			m["nutrition"].(map[string]any)["deepCutBfThreshold"] = 60.0
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := validPlan()
			tt.mutate(body)

			rec := postJSON(t, planServer(&stubPlan{}), http.MethodPut, "/v1/plan", body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestPutPlan_フェーズの期間が重なったら422(t *testing.T) {
	t.Parallel()

	// **重なりを許すと、その日の目標ペースが一意に決まらない。**
	// 停滞判定の基準が変わってしまう
	body := validPlan()
	body["phases"] = []map[string]any{
		{"name": "A", "startsOn": "2026-10-01", "endsOn": "2026-10-31", "goalKgPerWeek": -0.5},
		{"name": "B", "startsOn": "2026-10-15", "endsOn": "2026-11-15", "goalKgPerWeek": -0.3},
	}

	rec := postJSON(t, planServer(&stubPlan{}), http.MethodPut, "/v1/plan", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}
