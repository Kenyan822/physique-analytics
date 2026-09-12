package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

func monthlyServer(p *stubPlan, s *stubSeries) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		p, s, &stubMealSets{}, &stubContests{}, &stubBlood{}))
}

func planWithBaseline() openapi.Plan {
	p := planWithPhase()
	h, w, bf, m := float32(175), float32(75), float32(20), "2026-09"
	p.HeightCm, p.BaselineWeightKg, p.BaselineBodyfatPct, p.BaselineMonth = &h, &w, &bf, &m

	return p
}

func sampleBlocks() []openapi.PlanBlock {
	return []openapi.PlanBlock{
		{Name: "P0 基盤", Months: 1, LbmDeltaKgPerMonth: 0.15, BodyfatPctEnd: 21.2},
		{Name: "P1-A カット", Months: 2, LbmDeltaKgPerMonth: -0.35, BodyfatPctEnd: 18.1},
	}
}

func TestPutPlanBlocks_入力を渡す(t *testing.T) {
	t.Parallel()

	stub := &stubPlan{}
	rec := postJSON(t, monthlyServer(stub, &stubSeries{}), http.MethodPut, "/v1/plan/blocks",
		map[string]any{"items": []map[string]any{
			{"name": "P0 基盤", "months": 1, "lbmDeltaKgPerMonth": 0.15, "bodyfatPctEnd": 21.2},
		}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if len(stub.gotBlocks) != 1 || stub.gotBlocks[0].Name != "P0 基盤" {
		t.Errorf("blocks = %+v", stub.gotBlocks)
	}
}

func TestPutPlanBlocks_検証(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		block map[string]any
	}{
		{"名前が空", map[string]any{"name": " ", "months": 1, "lbmDeltaKgPerMonth": 0, "bodyfatPctEnd": 20}},
		{"月数が0", map[string]any{"name": "x", "months": 0, "lbmDeltaKgPerMonth": 0, "bodyfatPctEnd": 20}},
		{"LBM の増減が大きすぎる", map[string]any{"name": "x", "months": 1, "lbmDeltaKgPerMonth": 5, "bodyfatPctEnd": 20}},
		{"体脂肪率が範囲外", map[string]any{"name": "x", "months": 1, "lbmDeltaKgPerMonth": 0, "bodyfatPctEnd": 0}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, monthlyServer(&stubPlan{}, &stubSeries{}), http.MethodPut,
				"/v1/plan/blocks", map[string]any{"items": []map[string]any{tt.block}})

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestGetMonthlyTargets_設定した起点から計算する(t *testing.T) {
	t.Parallel()

	stub := &stubPlan{plan: planWithBaseline(), blocks: sampleBlocks()}
	rec := httptest.NewRecorder()
	monthlyServer(stub, &stubSeries{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/plan/monthly-targets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.MonthlyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got.Baseline.Source != openapi.MonthlyTargetsBaselineSourceConfigured {
		t.Errorf("source = %q, want configured", got.Baseline.Source)
	}
	if len(got.Items) != 3 {
		t.Fatalf("月数 = %d, want 3", len(got.Items))
	}
	if got.Items[0].Month != "2026-09" {
		t.Errorf("先頭の月 = %q, want 2026-09", got.Items[0].Month)
	}
	// 起点 LBM 60.0 + 0.15
	if diff := got.Items[0].LbmKg - 60.15; diff > 0.01 || diff < -0.01 {
		t.Errorf("LBM = %.2f, want 60.15", got.Items[0].LbmKg)
	}
}

func TestGetMonthlyTargets_実測から引き直す(t *testing.T) {
	t.Parallel()

	// **要件 P-03。** 直近7日の実測を起点にする
	// 「直近7日」は今日から数えるので、今日を終端にした系列を作る
	series := &stubSeries{points: seriesWithBodyfat(21, timeutil.Now().Truncate(24*time.Hour), 73, 19, 2100)}
	stub := &stubPlan{plan: planWithBaseline(), blocks: sampleBlocks()}

	rec := httptest.NewRecorder()
	monthlyServer(stub, series).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/plan/monthly-targets?baseline=measured", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.MonthlyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got.Baseline.Source != openapi.MonthlyTargetsBaselineSourceMeasured {
		t.Errorf("source = %q, want measured", got.Baseline.Source)
	}
	// 設定の 75kg ではなく実測から始まる
	if got.Baseline.WeightKg > 74 {
		t.Errorf("baseline.weightKg = %.1f, want 実測（73前後）", got.Baseline.WeightKg)
	}
}

func TestGetMonthlyTargets_実測が無ければ422(t *testing.T) {
	t.Parallel()

	// **設定値へ黙って落ちない。** どちらを見ているか分からないまま
	// 数字を読むと判断を誤る
	stub := &stubPlan{plan: planWithBaseline(), blocks: sampleBlocks()}
	rec := httptest.NewRecorder()
	monthlyServer(stub, &stubSeries{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/plan/monthly-targets?baseline=measured", nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestGetMonthlyTargets_ブロックが無ければ422(t *testing.T) {
	t.Parallel()

	stub := &stubPlan{plan: planWithBaseline()}
	rec := httptest.NewRecorder()
	monthlyServer(stub, &stubSeries{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/plan/monthly-targets", nil))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
	if !jsonContains(rec.Body.String(), "設定画面") {
		t.Errorf("body = %s, want 直し方を含む", rec.Body)
	}
}

// seriesWithBodyfat は体脂肪率つきの日次記録を作る。
func seriesWithBodyfat(n int, d time.Time, weight, bf, kcal float64) []analytics.DailyPoint {
	out := make([]analytics.DailyPoint, 0, n)
	for i := range n {
		w := weight + 0.06*float64(i)
		b := bf
		k := kcal
		out = append(out, analytics.DailyPoint{
			Date: d.AddDate(0, 0, -i), WeightKg: &w, BodyfatPct: &b, Kcal: &k,
		})
	}

	return out
}
