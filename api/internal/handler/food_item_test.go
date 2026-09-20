package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
)

// stubFoodItems は食品マスタの差し替え。
type stubFoodItems struct {
	items   []openapi.FoodItem
	created openapi.FoodItem
	gotIn   *openapi.FoodItemInput
	gotQ    string
	gotID   uuid.UUID
	deleted bool
	used    bool
}

func (s *stubFoodItems) List(_ context.Context, q string) ([]openapi.FoodItem, error) {
	s.gotQ = q

	return s.items, nil
}

func (s *stubFoodItems) Get(_ context.Context, id uuid.UUID) (openapi.FoodItem, error) {
	s.gotID = id

	return s.created, nil
}

func (s *stubFoodItems) Create(_ context.Context, in openapi.FoodItemInput) (openapi.FoodItem, error) {
	s.gotIn = &in

	return s.created, nil
}

func (s *stubFoodItems) Update(_ context.Context, id uuid.UUID, in openapi.FoodItemInput) (openapi.FoodItem, error) {
	s.gotID, s.gotIn = id, &in

	return s.created, nil
}

func (s *stubFoodItems) Delete(_ context.Context, id uuid.UUID) error {
	s.gotID, s.deleted = id, true

	return nil
}

func (s *stubFoodItems) MarkUsed(_ context.Context, id uuid.UUID) error {
	s.gotID, s.used = id, true

	return nil
}

func foodServer(f *stubFoodItems) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{},
		&stubEstimator{}, &stubPhotos{}, &stubBlobs{})

	return handler.NewRouter(srv.WithFoodItems(f))
}

func TestListFoodItems_一覧を返す(t *testing.T) {
	t.Parallel()

	stub := &stubFoodItems{items: []openapi.FoodItem{{Id: uuid.New(), Name: "プロテイン"}}}
	rec := postJSON(t, foodServer(stub), http.MethodGet, "/v1/food-items", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var body struct {
		Items []openapi.FoodItem `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 1 {
		t.Errorf("items = %d件", len(body.Items))
	}
}

func TestListFoodItems_未設定なら空配列(t *testing.T) {
	t.Parallel()

	// **null を返さない。** クライアントが map する前提
	rec := postJSON(t, foodServer(&stubFoodItems{}), http.MethodGet, "/v1/food-items", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if !jsonContains(rec.Body.String(), `"items":[]`) {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestCreateFoodItem_引数なしで登録できる(t *testing.T) {
	t.Parallel()

	// **基本は引数なし**（ADR-0017）
	stub := &stubFoodItems{created: openapi.FoodItem{Id: uuid.New(), Name: "プロテイン"}}
	rec := postJSON(t, foodServer(stub), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"プロテイン","qty":"1杯","proteinG":24,"fatG":1.5,"carbG":2}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotIn == nil || stub.gotIn.Name != "プロテイン" {
		t.Errorf("Name = %v", stub.gotIn)
	}
}

func TestCreateFoodItem_引数つきで登録できる(t *testing.T) {
	t.Parallel()

	stub := &stubFoodItems{created: openapi.FoodItem{Id: uuid.New()}}
	rec := postJSON(t, foodServer(stub), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"プロテイン","components":[
			{"name":"量","unit":"g","basisAmount":30,"defaultAmount":30,"proteinG":24}]}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotIn.Components == nil || len(*stub.gotIn.Components) != 1 {
		t.Fatalf("Components = %v", stub.gotIn.Components)
	}
	if (*stub.gotIn.Components)[0].BasisAmount != 30 {
		t.Errorf("BasisAmount = %v", (*stub.gotIn.Components)[0].BasisAmount)
	}
}

func TestCreateFoodItem_名前が空なら422(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, foodServer(&stubFoodItems{}), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"  "}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateFoodItem_基準量が0なら422(t *testing.T) {
	t.Parallel()

	// **0 では割れない。** DB の check より手前で弾いて、理由を返す
	rec := postJSON(t, foodServer(&stubFoodItems{}), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"x","components":[
			{"name":"量","basisAmount":0,"defaultAmount":10}]}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateFoodItem_構成の名前が空なら422(t *testing.T) {
	t.Parallel()

	// 名前が無いと入力画面でどの欄か分からない
	rec := postJSON(t, foodServer(&stubFoodItems{}), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"x","components":[
			{"name":" ","basisAmount":30,"defaultAmount":30}]}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateFoodItem_構成の名前が重複したら422(t *testing.T) {
	t.Parallel()

	// **入力量は名前で引く。** 重複すると片方しか届かない
	rec := postJSON(t, foodServer(&stubFoodItems{}), http.MethodPost, "/v1/food-items",
		json.RawMessage(`{"name":"x","components":[
			{"name":"量","basisAmount":30,"defaultAmount":30},
			{"name":"量","basisAmount":10,"defaultAmount":10}]}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestDeleteFoodItem_消せる(t *testing.T) {
	t.Parallel()

	stub := &stubFoodItems{}
	id := uuid.New()
	rec := postJSON(t, foodServer(stub), http.MethodDelete, "/v1/food-items/"+id.String(), nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body)
	}
	if !stub.deleted {
		t.Error("消していない")
	}
}
