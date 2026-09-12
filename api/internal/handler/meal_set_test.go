package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubMealSets struct {
	items []openapi.MealSet

	gotInput *openapi.MealSetInput
	gotID    uuid.UUID
	gotDate  openapi_types.Date
	gotSlot  *openapi.MealSlot

	created   openapi.MealSet
	applied   []openapi.Meal
	createErr error
	updateErr error
	deleteErr error
	applyErr  error
}

func (s *stubMealSets) List(context.Context) ([]openapi.MealSet, error) { return s.items, nil }

func (s *stubMealSets) Create(_ context.Context, in openapi.MealSetInput) (openapi.MealSet, error) {
	s.gotInput = &in
	return s.created, s.createErr
}

func (s *stubMealSets) Update(_ context.Context, id uuid.UUID, in openapi.MealSetInput) (openapi.MealSet, error) {
	s.gotID, s.gotInput = id, &in
	return s.created, s.updateErr
}

func (s *stubMealSets) SoftDelete(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func (s *stubMealSets) Apply(_ context.Context, id uuid.UUID, date openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error) {
	s.gotID, s.gotDate, s.gotSlot = id, date, slot
	return s.applied, s.applyErr
}

func mealSetServer(ms *stubMealSets) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, ms, &stubContests{}, &stubBlood{}, &stubEstimator{}))
}

func validSet() map[string]any {
	return map[string]any{
		"name": "朝食セット",
		"slot": "朝食",
		"items": []map[string]any{
			{"name": "オートミール", "qty": "80g", "kcal": 304, "proteinG": 11},
		},
	}
}

func TestCreateMealSet_項目つきで作れる(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{created: openapi.MealSet{Id: uuid.New(), Name: "朝食セット"}}
	rec := postJSON(t, mealSetServer(stub), http.MethodPost, "/v1/meal-sets", validSet())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput == nil || len(stub.gotInput.Items) != 1 {
		t.Fatalf("入力が渡っていない: %+v", stub.gotInput)
	}
	if stub.gotInput.Slot == nil || *stub.gotInput.Slot != openapi.Breakfast {
		t.Errorf("Slot = %v, want 朝食", stub.gotInput.Slot)
	}
}

func TestCreateMealSet_項目が空なら422(t *testing.T) {
	t.Parallel()

	// 空のセットは展開しても何も起きず、壊れて見える
	body := validSet()
	body["items"] = []map[string]any{}

	rec := postJSON(t, mealSetServer(&stubMealSets{}), http.MethodPost, "/v1/meal-sets", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateMealSet_項目名が空なら422(t *testing.T) {
	t.Parallel()

	body := validSet()
	body["items"] = []map[string]any{{"name": "  "}}

	rec := postJSON(t, mealSetServer(&stubMealSets{}), http.MethodPost, "/v1/meal-sets", body)
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateMealSet_名前が重複したら409(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{createErr: repository.ErrConflict}
	rec := postJSON(t, mealSetServer(stub), http.MethodPost, "/v1/meal-sets", validSet())

	if rec.Code != http.StatusConflict {
		t.Fatalf("status = %d, want 409, body = %s", rec.Code, rec.Body)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestUpdateMealSet_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{updateErr: repository.ErrNotFound}
	rec := postJSON(t, mealSetServer(stub), http.MethodPut,
		"/v1/meal-sets/"+uuid.New().String(), validSet())

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
	}
}

func TestDeleteMealSet_204(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubMealSets{}
	rec := httptest.NewRecorder()
	mealSetServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/meal-sets/"+id.String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if stub.gotID != id {
		t.Errorf("ID = %v, want %v", stub.gotID, id)
	}
}

func TestApplyMealSet_日付と区分を渡す(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{applied: []openapi.Meal{{Id: uuid.New(), Name: "オートミール"}}}
	rec := postJSON(t, mealSetServer(stub), http.MethodPost,
		"/v1/meal-sets/"+uuid.New().String()+"/apply",
		map[string]any{"date": "2033-03-01", "slot": "夕食"})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotDate.Format("2006-01-02") != "2033-03-01" {
		t.Errorf("date = %v", stub.gotDate)
	}
	if stub.gotSlot == nil || *stub.gotSlot != openapi.Dinner {
		t.Errorf("slot = %v, want 夕食", stub.gotSlot)
	}

	var got struct {
		Items []openapi.Meal `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(got.Items) != 1 {
		t.Errorf("items = %d 件, want 1", len(got.Items))
	}
}

func TestApplyMealSet_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{applyErr: repository.ErrNotFound}
	rec := postJSON(t, mealSetServer(stub), http.MethodPost,
		"/v1/meal-sets/"+uuid.New().String()+"/apply", map[string]any{"date": "2033-03-01"})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
	}
}

func TestListMealSets_空でもitemsはnullにしない(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	mealSetServer(&stubMealSets{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/meal-sets", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !jsonContains(rec.Body.String(), `"items":[]`) {
		t.Errorf("body = %s, want items:[]", rec.Body)
	}
}
