package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

// stubManualTargets は手動目標の差し替え。
type stubManualTargets struct {
	current *openapi.ManualTargets
	got     *openapi.ManualTargets
	deleted bool
}

func (s *stubManualTargets) Get(context.Context) (*openapi.ManualTargets, error) {
	return s.current, nil
}

func (s *stubManualTargets) Put(_ context.Context, in openapi.ManualTargets) (openapi.ManualTargets, error) {
	s.got = &in
	s.current = &in

	return in, nil
}

func (s *stubManualTargets) Delete(context.Context) error {
	s.deleted = true
	s.current = nil

	return nil
}

func manualServer(mt *stubManualTargets, p *stubPlan, se *stubSeries, m *stubMeals) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, m, p, se,
		&stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{})

	return handler.NewRouter(srv.WithManualTargets(mt))
}

func TestGetManualTargets_未設定ならnull(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/manual", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var body struct {
		Targets *openapi.ManualTargets `json:"targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Targets != nil {
		t.Errorf("targets = %+v, want null", body.Targets)
	}
}

func TestPutManualTargets_保存する(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{}
	rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodPut, "/v1/targets/manual",
		json.RawMessage(`{"proteinG":180,"fatG":70,"carbG":250}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.got == nil || stub.got.ProteinG != 180 {
		t.Errorf("保存されていない: %+v", stub.got)
	}
}

func TestPutManualTargets_範囲外は422(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"proteinG":-1,"fatG":70,"carbG":250}`,
		`{"proteinG":180,"fatG":70,"carbG":9999}`,
	} {
		rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
			http.MethodPut, "/v1/targets/manual", json.RawMessage(body))

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
		}
	}
}

func TestDeleteManualTargets_消せる(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{current: &openapi.ManualTargets{ProteinG: 180}}
	rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodDelete, "/v1/targets/manual", nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body)
	}
	if !stub.deleted {
		t.Error("消していない")
	}
}

func TestGetDailyTargets_手動目標があれば優先する(t *testing.T) {
	t.Parallel()

	// **フェーズが無くても目標が出る。** いままで 422 で何も出なかった
	mt := &stubManualTargets{current: &openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}}
	rec := postJSON(t, manualServer(mt, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/2026-11-01", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Target == nil || got.Target.ProteinG != 180 {
		t.Fatalf("Target = %+v", got.Target)
	}
	// **どちらの値かを返す。** 体重が動いても目標が変わらない理由が分かるように
	if got.TargetSource == nil || *got.TargetSource != openapi.DailyTargetsTargetSourceManual {
		t.Errorf("TargetSource = %v, want manual", got.TargetSource)
	}
	if got.Remaining == nil || got.Remaining.ProteinG != 180 {
		t.Errorf("Remaining = %+v", got.Remaining)
	}
}

func TestGetDailyTargets_手動が無ければ自動計算(t *testing.T) {
	t.Parallel()

	p := &stubPlan{plan: planWithPhase()}
	se := &stubSeries{points: series(21, "2026-11-01", 75, 2400)}
	rec := postJSON(t, manualServer(&stubManualTargets{}, p, se, &stubMeals{}),
		http.MethodGet, "/v1/targets/2026-11-01", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TargetSource == nil || *got.TargetSource != openapi.DailyTargetsTargetSourceComputed {
		t.Errorf("TargetSource = %v, want computed", got.TargetSource)
	}
}
