package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

// 仕様にないパスは 404。生成されたルータの既定動作が変わっていないことの確認。
func TestRouter_未定義のパスは404(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	srv := handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}))
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/unknown", nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusNotFound)
	}
}

// 定義されているパスでも、仕様にないメソッドは通さない
func TestRouter_未定義のメソッドは404(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	srv := handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}))
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v1/exercises", nil))

	if rec.Code != http.StatusNotFound && rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 404 か 405", rec.Code)
	}
}
