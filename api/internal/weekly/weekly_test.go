package weekly_test

import (
	"context"
	"math"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
)

var asof = time.Date(2026, 10, 31, 0, 0, 0, 0, time.UTC)

type stubPlan struct {
	plan   openapi.Plan
	blocks []openapi.PlanBlock
}

func (s *stubPlan) Get(context.Context) (openapi.Plan, error) { return s.plan, nil }

func (s *stubPlan) ListBlocks(context.Context) ([]openapi.PlanBlock, error) {
	return s.blocks, nil
}

type stubSeries struct {
	points []analytics.DailyPoint
}

func (s *stubSeries) DailySeries(context.Context, openapi_types.Date, openapi_types.Date) ([]analytics.DailyPoint, error) {
	return s.points, nil
}

func f32(v float64) *float32 { f := float32(v); return &f }

func plan() openapi.Plan {
	m := "2026-09"

	return openapi.Plan{
		HeightCm:           f32(175),
		BaselineWeightKg:   f32(75),
		BaselineBodyfatPct: f32(20),
		BaselineMonth:      &m,
		Phases: []openapi.PlanPhase{{
			Name:          "P1-A",
			StartsOn:      openapi_types.Date{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
			EndsOn:        openapi_types.Date{Time: time.Date(2026, 12, 31, 0, 0, 0, 0, time.UTC)},
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

func blocks() []openapi.PlanBlock {
	return []openapi.PlanBlock{
		{Name: "P0 基盤", Months: 1, LbmDeltaKgPerMonth: 0.15, BodyfatPctEnd: 21.2},
		{Name: "P1-A カット", Months: 2, LbmDeltaKgPerMonth: -0.35, BodyfatPctEnd: 18.1},
	}
}

// series は asof を終端に n 日ぶん作る。
func series(n int, weight, bf, kcal float64) []analytics.DailyPoint {
	out := make([]analytics.DailyPoint, 0, n)
	for i := range n {
		w := weight + 0.06*float64(i)
		b, k := bf, kcal
		out = append(out, analytics.DailyPoint{
			Date: asof.AddDate(0, 0, -i), WeightKg: &w, BodyfatPct: &b, Kcal: &k,
		})
	}

	return out
}

func TestBuild_体組成を出す(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Composition == nil {
		t.Fatal("Composition = nil")
	}
	// 7日平均の体重は 74 + 0.06×(0..6) の平均 = 74.18
	want := analytics.Lbm(74.18, 20)
	if math.Abs(got.Composition.LbmKg-want) > 0.01 {
		t.Errorf("LbmKg = %.2f, want %.2f", got.Composition.LbmKg, want)
	}
}

func TestBuild_体脂肪率が無ければ体組成を出さない(t *testing.T) {
	t.Parallel()

	points := series(21, 74, 20, 2100)
	for i := range points {
		points[i].BodyfatPct = nil
	}

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: points},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// **推定値を混ぜない。** 体組成計の誤差は ±3〜5% ある
	if got.Composition != nil {
		t.Errorf("Composition = %+v, want nil", got.Composition)
	}
}

func TestBuild_月次目標との乖離を出す(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan(), blocks: blocks()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Deviation == nil {
		t.Fatal("Deviation = nil")
	}
	if got.Deviation.Month != "2026-10" {
		t.Errorf("Month = %q, want 2026-10", got.Deviation.Month)
	}
	// 2026-10 の目標 LBM は 59.80。実測は 74.18 × 0.80 = 59.34 → -0.46
	if math.Abs(got.Deviation.LbmKg-(-0.46)) > 0.02 {
		t.Errorf("LbmKg = %.2f, want -0.46", got.Deviation.LbmKg)
	}
	if got.Deviation.LbmBehind {
		t.Error("LbmBehind = true, want false（-1kg 以内）")
	}
}

func TestBuild_ブロックが無ければ乖離を出さない(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 計画を立てていないのは正常な状態
	if got.Deviation != nil {
		t.Errorf("Deviation = %+v, want nil", got.Deviation)
	}
}

func TestBuild_LBMが遅れていればアクションに出る(t *testing.T) {
	t.Parallel()

	// 体重は目標付近でも、体脂肪率が高ければ LBM は足りない
	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan(), blocks: blocks()},
		Series: &stubSeries{points: series(21, 74, 24, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Deviation == nil || !got.Deviation.LbmBehind {
		t.Fatalf("LbmBehind でない: %+v", got.Deviation)
	}
	if !got.ActionContext().LbmBehind {
		t.Error("ActionContext に伝わっていない")
	}
}

type stubMeasurements struct {
	m   openapi.BodyMeasurement
	err error
}

func (s *stubMeasurements) LatestMeasurement(context.Context) (openapi.BodyMeasurement, error) {
	return s.m, s.err
}

func measurement(neck, shoulder, waist float64) openapi.BodyMeasurement {
	return openapi.BodyMeasurement{
		Date:         openapi_types.Date{Time: asof},
		NeckCm:       f32(neck),
		ShoulderCm:   f32(shoulder),
		WaistNavelCm: f32(waist),
	}
}

func TestBuild_周囲長から海軍式と肩ウエスト比を出す(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:         &stubPlan{plan: plan()},
		Series:       &stubSeries{points: series(21, 74, 20, 2100)},
		Measurements: &stubMeasurements{m: measurement(39, 120, 75)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	if got.Taper == nil {
		t.Fatal("Taper = nil")
	}
	if got.Taper.NavyBodyfatPct == nil {
		t.Error("NavyBodyfatPct = nil")
	}
	if got.Taper.ShoulderWaist == nil || !got.Taper.ShoulderWaist.VTaper {
		t.Errorf("ShoulderWaist = %+v, want VTaper（120/75 = 1.6）", got.Taper.ShoulderWaist)
	}
}

func TestBuild_周囲長が無ければ出さない(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:         &stubPlan{plan: plan()},
		Series:       &stubSeries{points: series(21, 74, 20, 2100)},
		Measurements: &stubMeasurements{err: repository.ErrNotFound},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 周囲長は週1回の記録。無い週があるのは正常
	if got.Taper != nil {
		t.Errorf("Taper = %+v, want nil", got.Taper)
	}
}

func TestBuild_首だけでは海軍式を出さない(t *testing.T) {
	t.Parallel()

	m := openapi.BodyMeasurement{Date: openapi_types.Date{Time: asof}, NeckCm: f32(39)}
	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:         &stubPlan{plan: plan()},
		Series:       &stubSeries{points: series(21, 74, 20, 2100)},
		Measurements: &stubMeasurements{m: m},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// どちらも出せないなら持たせない。日付だけ返しても読み手が困る
	if got.Taper != nil {
		t.Errorf("Taper = %+v, want nil", got.Taper)
	}
}
