package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// stubBody は handler.BodyRepository の差し替え。
// ハンドラで確かめたいのは「絞り込みと入力が正しく渡るか」と
// 「ErrNotFound が 404 + Problem になるか」で、SQL は repository のテストで見る。
type stubBody struct {
	daily        []openapi.DailyMetrics
	measurements []openapi.BodyMeasurement

	gotFrom, gotTo *openapi_types.Date
	gotDaily       *repository.DailyInput
	gotMeasurement *repository.MeasurementInput
	gotDate        openapi_types.Date

	putDailyReturns       openapi.DailyMetrics
	putMeasurementReturns openapi.BodyMeasurement
	latestReturns         openapi.BodyMeasurement
	getErr                error
	deleteErr             error
	latestErr             error
}

func (s *stubBody) ListDaily(_ context.Context, from, to *openapi_types.Date) ([]openapi.DailyMetrics, error) {
	s.gotFrom, s.gotTo = from, to
	return s.daily, nil
}

func (s *stubBody) GetDaily(_ context.Context, date openapi_types.Date) (openapi.DailyMetrics, error) {
	s.gotDate = date
	return s.putDailyReturns, s.getErr
}

func (s *stubBody) PutDaily(_ context.Context, in repository.DailyInput) (openapi.DailyMetrics, error) {
	s.gotDaily = &in
	return s.putDailyReturns, nil
}

func (s *stubBody) SoftDeleteDaily(_ context.Context, date openapi_types.Date) error {
	s.gotDate = date
	return s.deleteErr
}

func (s *stubBody) ListMeasurements(_ context.Context, from, to *openapi_types.Date) ([]openapi.BodyMeasurement, error) {
	s.gotFrom, s.gotTo = from, to
	return s.measurements, nil
}

func (s *stubBody) PutMeasurement(_ context.Context, in repository.MeasurementInput) (openapi.BodyMeasurement, error) {
	s.gotMeasurement = &in
	return s.putMeasurementReturns, nil
}

func (s *stubBody) LatestMeasurement(context.Context) (openapi.BodyMeasurement, error) {
	return s.latestReturns, s.latestErr
}

func bodyServer(b *stubBody) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, b, &stubMeals{}))
}

func TestListDailyMetrics_期間を渡す(t *testing.T) {
	t.Parallel()

	stub := &stubBody{daily: []openapi.DailyMetrics{{Date: mustDate(t, "2031-04-01")}}}
	rec := httptest.NewRecorder()
	bodyServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/daily?from=2031-04-01&to=2031-04-30", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotFrom == nil || stub.gotFrom.Format("2006-01-02") != "2031-04-01" {
		t.Errorf("from = %v, want 2031-04-01", stub.gotFrom)
	}
	if stub.gotTo == nil || stub.gotTo.Format("2006-01-02") != "2031-04-30" {
		t.Errorf("to = %v, want 2031-04-30", stub.gotTo)
	}

	var got struct {
		Items []openapi.DailyMetrics `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(got.Items) != 1 {
		t.Errorf("items = %d 件, want 1", len(got.Items))
	}
}

func TestListDailyMetrics_空でもitemsはnullにしない(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	bodyServer(&stubBody{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/daily", nil))

	// required: [items] を満たさなくなるため、null では返さない
	if got := rec.Body.String(); !bytes.Contains([]byte(got), []byte(`"items":[]`)) {
		t.Errorf("body = %s, want items:[]", got)
	}
}

func TestPutDailyMetrics_入力をそのまま渡す(t *testing.T) {
	t.Parallel()

	stub := &stubBody{}
	rec := httptest.NewRecorder()
	body := `{"date":"2031-04-01","weightKg":75.1,"kcal":2700,"fatigue":3}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v1/daily", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	bodyServer(stub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotDaily == nil {
		t.Fatal("PutDaily が呼ばれていない")
	}
	if got := stub.gotDaily.Date.Format("2006-01-02"); got != "2031-04-01" {
		t.Errorf("date = %s, want 2031-04-01", got)
	}
	if stub.gotDaily.WeightKg == nil || *stub.gotDaily.WeightKg != 75.1 {
		t.Errorf("weightKg = %v, want 75.1", stub.gotDaily.WeightKg)
	}
	// 送っていない項目は nil で渡る。repository 側で「変更しない」になる
	if stub.gotDaily.BodyfatPct != nil {
		t.Errorf("bodyfatPct = %v, want nil", stub.gotDaily.BodyfatPct)
	}
}

func TestPutDailyMetrics_範囲外は422(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	// 疲労度は 1-5。DB の check 制約に当てる前にハンドラで弾く
	body := `{"date":"2031-04-01","fatigue":9}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v1/daily", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	bodyServer(&stubBody{}).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestGetDailyMetrics_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubBody{getErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	bodyServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/daily/2031-04-01", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
}

func TestDeleteDailyMetrics_消せたら204(t *testing.T) {
	t.Parallel()

	stub := &stubBody{}
	rec := httptest.NewRecorder()
	bodyServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/daily/2031-04-01", nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body)
	}
	if got := stub.gotDate.Format("2006-01-02"); got != "2031-04-01" {
		t.Errorf("date = %s, want 2031-04-01", got)
	}
}

func TestPutMeasurement_入力をそのまま渡す(t *testing.T) {
	t.Parallel()

	stub := &stubBody{}
	rec := httptest.NewRecorder()
	body := `{"date":"2031-09-01","neckCm":39,"waistNavelCm":86}`
	req := httptest.NewRequestWithContext(t.Context(), http.MethodPut, "/v1/measurements", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	bodyServer(stub).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotMeasurement == nil {
		t.Fatal("PutMeasurement が呼ばれていない")
	}
	if stub.gotMeasurement.NeckCm == nil || *stub.gotMeasurement.NeckCm != 39 {
		t.Errorf("neckCm = %v, want 39", stub.gotMeasurement.NeckCm)
	}
}

func TestGetLatestMeasurement_記録が無ければnull(t *testing.T) {
	t.Parallel()

	stub := &stubBody{latestErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	bodyServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/measurements/latest", nil))

	// **404 にしない。** 初回は記録が無いのが正常で、Web 側で
	// エラー扱いにすると入力画面が開けなくなる（要件 B-03）
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200, body = %s", rec.Code, rec.Body)
	}
	var got map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if v, ok := got["measurement"]; ok && v != nil {
		t.Errorf("measurement = %v, want null", v)
	}
}

func mustDate(t *testing.T, s string) openapi_types.Date {
	t.Helper()

	var d openapi_types.Date
	if err := d.UnmarshalText([]byte(s)); err != nil {
		t.Fatalf("日付 %q: %v", s, err)
	}

	return d
}
