package handler_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

type stubPinger struct{ err error }

func (p stubPinger) Ping(context.Context) error { return p.err }

// newTestServer は生成されたルータ経由でハンドラを組み立てる。
// ハンドラのメソッドを直接呼ぶとルーティングと Content-Type の検証が抜けるため、
// openapi.yaml から生成された配線をそのまま通す。
func newTestServer(t *testing.T, p handler.Pinger) http.Handler {
	t.Helper()
	return handler.NewRouter(handler.New(p, &stubExercises{}, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}))
}

func TestGetHealth_DBが応答すれば200を返す(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	newTestServer(t, stubPinger{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusOK, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/json")
	}

	var body struct {
		Status string    `json:"status"`
		Time   time.Time `json:"time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("レスポンスが JSON として読めない: %v (body=%s)", err, rec.Body.String())
	}
	if body.Status != "ok" {
		t.Errorf("status = %q, want %q", body.Status, "ok")
	}
}

// ADR-0013: 時刻はすべて JST で扱う。サーバの TZ 設定に依存してはいけない。
func TestGetHealth_時刻をJSTで返す(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	newTestServer(t, stubPinger{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	var body struct {
		Time string `json:"time"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("レスポンスが JSON として読めない: %v", err)
	}

	ts, err := time.Parse(time.RFC3339Nano, body.Time)
	if err != nil {
		t.Fatalf("time が RFC3339 ではない: %v (time=%q)", err, body.Time)
	}
	if _, offset := ts.Zone(); offset != 9*60*60 {
		t.Errorf("UTC オフセット = %d秒, want %d秒 (JST) / time=%q", offset, 9*60*60, body.Time)
	}
	if d := time.Since(ts); d < -time.Minute || d > time.Minute {
		t.Errorf("現在時刻から %v ずれている (time=%q)", d, body.Time)
	}
}

func TestGetHealth_DBが応答しなければ503とProblemを返す(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	srv := newTestServer(t, stubPinger{err: errors.New("connection refused")})
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
	// RFC 7807。エラー形式は openapi.yaml の Problem に統一している
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}

	var problem openapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("レスポンスが JSON として読めない: %v (body=%s)", err, rec.Body.String())
	}
	if problem.Status != http.StatusServiceUnavailable {
		t.Errorf("problem.status = %d, want %d", problem.Status, http.StatusServiceUnavailable)
	}
	if problem.Title == "" {
		t.Error("problem.title が空")
	}
	// DB のエラー文字列をそのまま外に出すと接続先の情報が漏れる
	if problem.Detail != nil && *problem.Detail == "connection refused" {
		t.Errorf("DB のエラーがそのまま露出している: %q", *problem.Detail)
	}
}
