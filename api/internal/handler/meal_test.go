package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// stubMeals は handler.MealRepository の差し替え。
type stubMeals struct {
	items       []openapi.Meal
	suggestions []openapi.MealSuggestion
	copied      []openapi.Meal

	gotInput *repository.MealInput
	gotID    uuid.UUID
	gotQuery string
	gotLimit int
	gotFrom  openapi_types.Date
	gotTo    openapi_types.Date
	gotSlot  *openapi.MealSlot

	created   openapi.Meal
	updateErr error
	deleteErr error
}

func (s *stubMeals) List(context.Context, *openapi_types.Date, *openapi_types.Date) ([]openapi.Meal, error) {
	return s.items, nil
}

func (s *stubMeals) Create(_ context.Context, in repository.MealInput) (openapi.Meal, error) {
	s.gotInput = &in
	return s.created, nil
}

func (s *stubMeals) Update(_ context.Context, id uuid.UUID, in repository.MealInput) (openapi.Meal, error) {
	s.gotID, s.gotInput = id, &in
	return s.created, s.updateErr
}

func (s *stubMeals) SoftDelete(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

func (s *stubMeals) Suggestions(_ context.Context, q string, limit int) ([]openapi.MealSuggestion, error) {
	s.gotQuery, s.gotLimit = q, limit
	return s.suggestions, nil
}

func (s *stubMeals) Copy(_ context.Context, from, to openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error) {
	s.gotFrom, s.gotTo, s.gotSlot = from, to, slot
	return s.copied, nil
}

func mealServer(m *stubMeals) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, m, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
}

func TestCreateMeal_入力を渡して201(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{created: openapi.Meal{Id: uuid.New(), Name: ptr("サラダチキン")}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"サラダチキン","slot":"昼食","kcal":114,"proteinG":24.1}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput == nil {
		t.Fatal("Create が呼ばれていない")
	}
	if stub.gotInput.Name == nil || *stub.gotInput.Name != "サラダチキン" {
		t.Errorf("Name = %v", stub.gotInput.Name)
	}
	if stub.gotInput.Slot == nil || *stub.gotInput.Slot != openapi.Lunch {
		t.Errorf("Slot = %v, want 昼食", stub.gotInput.Slot)
	}
}

func TestCreateMeal_名前が200文字を超えたら422(t *testing.T) {
	t.Parallel()

	// 名前そのものは任意になった（#188）。長すぎるものだけ弾く
	long := strings.Repeat("あ", 201)
	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"`+long+`"}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateMeal_範囲外は422(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","kcal":99999}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestUpdateMeal_無ければ404(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{updateErr: repository.ErrNotFound}
	rec := postJSON(t, mealServer(stub), http.MethodPatch, "/v1/meals/"+uuid.New().String(),
		json.RawMessage(`{"date":"2032-05-01","name":"x"}`))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q", ct)
	}
}

func TestDeleteMeal_204(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubMeals{}
	rec := httptest.NewRecorder()
	mealServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/meals/"+id.String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if stub.gotID != id {
		t.Errorf("ID = %v, want %v", stub.gotID, id)
	}
}

func TestListMealSuggestions_既定の件数(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{}
	rec := httptest.NewRecorder()
	mealServer(stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/meals/suggestions?q=チキン", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotQuery != "チキン" {
		t.Errorf("q = %q, want チキン", stub.gotQuery)
	}
	if stub.gotLimit != 20 {
		t.Errorf("limit = %d, want 20（既定）", stub.gotLimit)
	}
	if !bytes.Contains(rec.Body.Bytes(), []byte(`"items":[]`)) {
		t.Errorf("body = %s, want items:[]", rec.Body)
	}
}

func TestCopyMeals_日付とslotを渡す(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{copied: []openapi.Meal{{Id: uuid.New(), Name: ptr("x")}}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals/copy",
		json.RawMessage(`{"fromDate":"2032-05-10","toDate":"2032-05-11","slot":"朝食"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotFrom.Format("2006-01-02") != "2032-05-10" {
		t.Errorf("from = %v", stub.gotFrom)
	}
	if stub.gotSlot == nil || *stub.gotSlot != openapi.Breakfast {
		t.Errorf("slot = %v, want 朝食", stub.gotSlot)
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

func TestCopyMeals_同じ日付なら400(t *testing.T) {
	t.Parallel()

	// 同じ日に複製すると倍になるだけで、意図した操作ではありえない
	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals/copy",
		json.RawMessage(`{"fromDate":"2032-05-10","toDate":"2032-05-10"}`))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400, body = %s", rec.Code, rec.Body)
	}
}

// --- enum の検証（#141） ---
//
// enum 外の値は DB の check 制約まで届いて 500 になっていた。
// 500 は「こちらの落ち度」を意味するので、入力ミスがこれになると
// 原因の切り分けができない。

func TestCreateMeal_enum外のslotは422(t *testing.T) {
	t.Parallel()

	// "昼" は enum に無い（正しくは "昼食"）
	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","slot":"昼"}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
	if !jsonContains(rec.Body.String(), "slot") {
		t.Errorf("body = %s, want どの項目が不正か分かること", rec.Body)
	}
}

func TestCreateMeal_enum外のsourceは422(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","source":"guess"}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateMeal_正しいslotは通る(t *testing.T) {
	t.Parallel()

	for _, slot := range []string{"朝食", "昼食", "夕食", "間食"} {
		rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
			json.RawMessage(`{"date":"2032-05-01","name":"x","slot":"`+slot+`"}`))

		if rec.Code != http.StatusCreated {
			t.Errorf("slot=%s: status = %d, want 201, body = %s", slot, rec.Code, rec.Body)
		}
	}
}

func TestCopyMeals_enum外のslotは弾く(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals/copy",
		json.RawMessage(`{"fromDate":"2032-05-01","toDate":"2032-05-02","slot":"昼"}`))

	if rec.Code == http.StatusInternalServerError {
		t.Fatalf("500 になっている（入力ミスを「こちらの落ち度」にしない）: %s", rec.Body)
	}
	if rec.Code < 400 || rec.Code >= 500 {
		t.Errorf("status = %d, want 4xx, body = %s", rec.Code, rec.Body)
	}
}

func TestApplyMealSet_enum外のslotは422(t *testing.T) {
	t.Parallel()

	stub := &stubMealSets{}
	rec := postJSON(t, mealSetServer(stub), http.MethodPost,
		"/v1/meal-sets/"+uuid.New().String()+"/apply",
		json.RawMessage(`{"date":"2032-05-01","slot":"昼"}`))

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestCreateMeal_時刻から区分を導出する(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		at   string
		want openapi.MealSlot
	}{
		{"朝", "07:20", openapi.Breakfast},
		{"昼", "12:15", openapi.Lunch},
		{"夕", "19:40", openapi.Dinner},
		{"深夜は間食", "03:00", openapi.Snack},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
			body := json.RawMessage(
				`{"date":"2032-05-01","name":"x","at":"` + tt.at + `"}`)
			rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals", body)

			if rec.Code != http.StatusCreated {
				t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
			}
			if stub.gotInput.At == nil || *stub.gotInput.At != tt.at {
				t.Errorf("At = %v, want %s", stub.gotInput.At, tt.at)
			}
			if stub.gotInput.Slot == nil || *stub.gotInput.Slot != tt.want {
				t.Errorf("Slot = %v, want %v", stub.gotInput.Slot, tt.want)
			}
		})
	}
}

func TestCreateMeal_明示した区分が優先される(t *testing.T) {
	t.Parallel()

	// **Web と食事セットは区分を明示して送る。** 時刻から上書きすると既存の動作が壊れる
	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","at":"19:40","slot":"間食"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Slot == nil || *stub.gotInput.Slot != openapi.Snack {
		t.Errorf("Slot = %v, want 間食", stub.gotInput.Slot)
	}
}

func TestCreateMeal_時刻が無ければ区分も無いまま(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Slot != nil {
		t.Errorf("Slot = %v, want nil", stub.gotInput.Slot)
	}
}

func TestCreateMeal_不正な時刻は422(t *testing.T) {
	t.Parallel()

	// **黙って無視しない。** 時刻を送ったのに落とされると、区分がずれた理由が分からない
	for _, at := range []string{"25:00", "12:60", "1940", "19:40:00", "9:40"} {
		t.Run(at, func(t *testing.T) {
			t.Parallel()

			rec := postJSON(t, mealServer(&stubMeals{}), http.MethodPost, "/v1/meals",
				json.RawMessage(`{"date":"2032-05-01","name":"x","at":"`+at+`"}`))

			if rec.Code != http.StatusUnprocessableEntity {
				t.Errorf("at=%q: status = %d, want 422, body = %s", at, rec.Code, rec.Body)
			}
		})
	}
}

func TestCreateMeal_PFCからkcalを計算する(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","proteinG":30,"fatG":10,"carbG":60}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	// 30*4 + 10*9 + 60*4 = 450
	if stub.gotInput.Kcal == nil || *stub.gotInput.Kcal != 450 {
		t.Errorf("Kcal = %v, want 450", stub.gotInput.Kcal)
	}
}

func TestCreateMeal_明示したkcalが優先される(t *testing.T) {
	t.Parallel()

	// **AI 推定（N-06）は kcal を直接持ってくる。** 計算値で上書きしない
	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","kcal":500,"proteinG":30,"fatG":10,"carbG":60}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Kcal == nil || *stub.gotInput.Kcal != 500 {
		t.Errorf("Kcal = %v, want 500", stub.gotInput.Kcal)
	}
}

func TestCreateMeal_欠けたPFCは0として計算する(t *testing.T) {
	t.Parallel()

	// **空欄は 0 とみなす。** 鶏むねの C のように「本当に 0」で
	// 空のまま済ませる場面が普通にある。クライアントの表示と同じ規則にする
	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"x","proteinG":30,"fatG":10}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	// 30*4 + 10*9 = 210
	if stub.gotInput.Kcal == nil || *stub.gotInput.Kcal != 210 {
		t.Errorf("Kcal = %v, want 210", stub.gotInput.Kcal)
	}
}

func TestCreateMeal_PFCが1つも無ければkcalを計算しない(t *testing.T) {
	t.Parallel()

	// 名前だけの記録に 0 kcal を付けない
	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"外食","at":"19:40"}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Kcal != nil {
		t.Errorf("Kcal = %v, want nil", stub.gotInput.Kcal)
	}
}

func TestCreateMeal_名前が無くても記録できる(t *testing.T) {
	t.Parallel()

	// **PFC だけ入れて済ませたい**（#188）。名前は後から足せる
	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","at":"19:40","proteinG":30,"fatG":10,"carbG":60}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Name != nil {
		t.Errorf("Name = %v, want nil", stub.gotInput.Name)
	}
}

func TestCreateMeal_空白だけの名前は無しとして扱う(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
	rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
		json.RawMessage(`{"date":"2032-05-01","name":"   ","proteinG":1,"fatG":1,"carbG":1}`))

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201, body = %s", rec.Code, rec.Body)
	}
	if stub.gotInput.Name != nil {
		t.Errorf("Name = %v, want nil", stub.gotInput.Name)
	}
}

func ptr[T any](v T) *T { return &v }
