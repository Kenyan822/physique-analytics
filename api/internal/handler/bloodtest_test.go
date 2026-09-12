package handler_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubBlood struct {
	items     []openapi.BloodTest
	gotInput  *openapi.BloodTestInput
	gotID     uuid.UUID
	created   openapi.BloodTest
	createErr error
	updateErr error
	deleteErr error
}

func (s *stubBlood) List(context.Context) ([]openapi.BloodTest, error) { return s.items, nil }

func (s *stubBlood) Create(_ context.Context, in openapi.BloodTestInput) (openapi.BloodTest, error) {
	s.gotInput = &in
	return s.created, s.createErr
}

func (s *stubBlood) Update(_ context.Context, id uuid.UUID, in openapi.BloodTestInput) (openapi.BloodTest, error) {
	s.gotID, s.gotInput = id, &in
	return s.created, s.updateErr
}

func (s *stubBlood) SoftDelete(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func bloodServer(b *stubBlood) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, b, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
}

func validBloodTest() map[string]any {
	return map[string]any{
		"date":   "2034-04-01",
		"clinic": "テストクリニック",
		"items": []map[string]any{
			{"name": "ヘモグロビン", "value": 15.2, "unit": "g/dL", "refLow": 13.0, "refHigh": 17.0},
		},
	}
}

func TestCreateBloodTest_登録できる(t *testing.T) {
	t.Parallel()

	stub := &stubBlood{created: openapi.BloodTest{Id: uuid.New()}}
	rec := postJSON(t, bloodServer(stub), http.MethodPost, "/v1/blood-tests", validBloodTest())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput == nil || len(stub.gotInput.Items) != 1 {
		t.Errorf("入力が渡っていない: %+v", stub.gotInput)
	}
}

func TestCreateBloodTest_検証(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		mutate func(map[string]any)
	}{
		{"項目が空", func(m map[string]any) { m["items"] = []map[string]any{} }},
		{"項目名が空", func(m map[string]any) {
			m["items"] = []map[string]any{{"name": "  "}}
		}},
		{"基準範囲が逆", func(m map[string]any) {
			m["items"] = []map[string]any{{"name": "x", "refLow": 17.0, "refHigh": 13.0}}
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			body := validBloodTest()
			tt.mutate(body)

			rec := postJSON(t, bloodServer(&stubBlood{}), http.MethodPost, "/v1/blood-tests", body)
			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
			}
		})
	}
}

func TestCreateBloodTest_同じ日は409(t *testing.T) {
	t.Parallel()

	stub := &stubBlood{createErr: repository.ErrConflict}
	rec := postJSON(t, bloodServer(stub), http.MethodPost, "/v1/blood-tests", validBloodTest())

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409, body = %s", rec.Code, rec.Body)
	}
}

func TestUpdateBloodTest_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubBlood{updateErr: repository.ErrNotFound}
	rec := postJSON(t, bloodServer(stub), http.MethodPut,
		"/v1/blood-tests/"+uuid.New().String(), validBloodTest())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
	}
}

func TestDeleteBloodTest_204(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubBlood{}
	rec := httptest.NewRecorder()
	bloodServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/blood-tests/"+id.String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if stub.gotID != id {
		t.Errorf("ID = %v", stub.gotID)
	}
}

func TestListBloodTests_空でもitemsはnullにしない(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	bloodServer(&stubBlood{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/blood-tests", nil))

	if !jsonContains(rec.Body.String(), `"items":[]`) {
		t.Errorf("body = %s, want items:[]", rec.Body)
	}
}
