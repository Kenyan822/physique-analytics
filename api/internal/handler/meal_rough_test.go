package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
)

// ざっくり入力（#252）。値はすべて架空
func TestCreateMeal_ざっくり入力(t *testing.T) {
	t.Parallel()

	rough := openapi.MealSourceRough
	manual := openapi.MealSourceManual

	tests := []struct {
		name string
		body string
		want int
		// 200系のとき、保存に渡された入力の期待値
		kcal    *int
		p, f, c *float32
		source  *openapi.MealSource
	}{
		{
			name: "rough で kcal だけなら P20/F30/C50 で按分する",
			body: `{"date":"2032-05-01","kcal":1000,"source":"rough"}`,
			want: http.StatusCreated,
			kcal: ptr(1000), p: ptr(float32(50)), f: ptr(float32(33.3)), c: ptr(float32(125)),
			source: &rough,
		},
		{
			name: "rough でも PFC が1つでもあれば按分しない（手で直した値を守る）",
			body: `{"date":"2032-05-01","kcal":1000,"proteinG":30,"source":"rough"}`,
			want: http.StatusCreated,
			kcal: ptr(1000), p: ptr(float32(30)), f: nil, c: nil,
			source: &rough,
		},
		{
			name: "manual で kcal だけなら按分しない",
			body: `{"date":"2032-05-01","kcal":1000,"source":"manual"}`,
			want: http.StatusCreated,
			kcal: ptr(1000), p: nil, f: nil, c: nil,
			source: &manual,
		},
		{
			name: "rough で kcal も PFC も無ければ422",
			body: `{"date":"2032-05-01","name":"飲み会","source":"rough"}`,
			want: http.StatusUnprocessableEntity,
		},
		{
			name: "知らない source は422",
			body: `{"date":"2032-05-01","kcal":1000,"source":"guess"}`,
			want: http.StatusUnprocessableEntity,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubMeals{created: openapi.Meal{Id: uuid.New()}}
			rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals", json.RawMessage(tt.body))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want != http.StatusCreated {
				if stub.gotInput != nil {
					t.Error("エラーなのに保存された")
				}

				return
			}

			got := stub.gotInput
			if got == nil {
				t.Fatal("Create が呼ばれていない")
			}
			assertPtr(t, "Kcal", got.Kcal, tt.kcal)
			assertPtr(t, "ProteinG", got.ProteinG, tt.p)
			assertPtr(t, "FatG", got.FatG, tt.f)
			assertPtr(t, "CarbG", got.CarbG, tt.c)
			assertPtr(t, "Source", got.Source, tt.source)
		})
	}
}

// 按分で決めた PFC を手で直せる（既存の更新でそのまま通る）
func TestUpdateMeal_ざっくり入力を手で直せる(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubMeals{created: openapi.Meal{Id: id}}
	rec := postJSON(t, mealServer(stub), http.MethodPatch, "/v1/meals/"+id.String(),
		json.RawMessage(`{"date":"2032-05-01","kcal":1000,"proteinG":80,"fatG":20,"carbG":125,"source":"rough"}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	var p float32 = 80
	assertPtr(t, "ProteinG", stub.gotInput.ProteinG, &p)
}

func assertPtr[T comparable](t *testing.T, name string, got, want *T) {
	t.Helper()

	switch {
	case got == nil && want == nil:
	case got == nil || want == nil:
		t.Errorf("%s = %v, want %v", name, deref(got), deref(want))
	case *got != *want:
		t.Errorf("%s = %v, want %v", name, *got, *want)
	}
}

func deref[T any](p *T) any {
	if p == nil {
		return nil
	}

	return *p
}
