package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

// openapi.yaml に定義済みで未実装の操作は 501 で返す。
// 生成されたデフォルトのエラーハンドラは text/plain の 500 を返すので、
// 差し替えが効いていることをここで担保する。
func TestUnimplemented_未実装の操作は501とProblemを返す(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	srv := handler.NewRouter(handler.New(stubPinger{}))
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/exercises", nil))

	if rec.Code != http.StatusNotImplemented {
		t.Fatalf("status = %d, want %d (body=%s)", rec.Code, http.StatusNotImplemented, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want %q", got, "application/problem+json")
	}

	var problem openapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("レスポンスが JSON として読めない: %v (body=%s)", err, rec.Body.String())
	}
	if problem.Status != http.StatusNotImplemented {
		t.Errorf("problem.status = %d, want %d", problem.Status, http.StatusNotImplemented)
	}
}

// 仕様にないパスは 404。生成されたルータの既定動作が変わっていないことの確認。
func TestRouter_未定義のパスは404(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	srv := handler.NewRouter(handler.New(stubPinger{}))
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/unknown", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}
