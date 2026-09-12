package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubContests struct {
	items []openapi.Contest
	next  openapi.Contest

	gotInput *openapi.ContestInput
	gotID    uuid.UUID

	created   openapi.Contest
	nextErr   error
	updateErr error
	deleteErr error
}

func (s *stubContests) List(context.Context) ([]openapi.Contest, error) { return s.items, nil }

func (s *stubContests) Next(context.Context, openapi_types.Date) (openapi.Contest, error) {
	if s.nextErr != nil {
		return openapi.Contest{}, s.nextErr
	}

	return s.next, nil
}

func (s *stubContests) Create(_ context.Context, in openapi.ContestInput) (openapi.Contest, error) {
	s.gotInput = &in
	return s.created, nil
}

func (s *stubContests) Update(_ context.Context, id uuid.UUID, in openapi.ContestInput) (openapi.Contest, error) {
	s.gotID, s.gotInput = id, &in
	return s.created, s.updateErr
}

func (s *stubContests) SoftDelete(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func contestServer(c *stubContests) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, c, &stubBlood{}))
}

func validContest() map[string]any {
	return map[string]any{
		"heldOn":      "2027-05-31",
		"category":    "サマスタ スタイリッシュガイ",
		"targetBfPct": 11.0,
		"goal":        "完走・経験",
	}
}

func TestCreateContest_登録できる(t *testing.T) {
	t.Parallel()

	stub := &stubContests{created: openapi.Contest{Id: uuid.New(), Category: "サマスタ"}}
	rec := postJSON(t, contestServer(stub), http.MethodPost, "/v1/contests", validContest())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput == nil || stub.gotInput.TargetBfPct != 11.0 {
		t.Errorf("入力が渡っていない: %+v", stub.gotInput)
	}
}

func TestCreateContest_検証(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"カテゴリが空", func(m map[string]any) { m["category"] = "  " }},
		{"目標体脂肪率が0", func(m map[string]any) { m["targetBfPct"] = 0 }},
		{"目標体脂肪率が50以上", func(m map[string]any) { m["targetBfPct"] = 55 }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := validContest()
			tt.mutate(body)

			rec := postJSON(t, contestServer(&stubContests{}), http.MethodPost, "/v1/contests", body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestUpdateContest_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubContests{updateErr: repository.ErrNotFound}
	rec := postJSON(t, contestServer(stub), http.MethodPatch,
		"/v1/contests/"+uuid.New().String(), validContest())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
	}
}

func TestDeleteContest_204(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubContests{}
	rec := httptest.NewRecorder()
	contestServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/contests/"+id.String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if stub.gotID != id {
		t.Errorf("ID = %v", stub.gotID)
	}
}

func TestListContests_空でもitemsはnullにしない(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	contestServer(&stubContests{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/contests", nil))

	if !jsonContains(rec.Body.String(), `"items":[]`) {
		t.Errorf("body = %s, want items:[]", rec.Body)
	}
}
