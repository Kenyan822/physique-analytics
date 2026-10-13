// 内部テストにしているのは、logMeal を MCP のセッションを張らずに呼ぶため。
// weekly_test.go と同じ理由。
package mcpserver

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// mealFixture はトランザクション内で動く Deps と、確認用の Meals を返す。
func mealFixture(t *testing.T) (Deps, *repository.Meal) {
	t.Helper()

	tx := testdb.Begin(t)
	meals := repository.NewMeal(tx)

	return Deps{Meals: meals}, meals
}

func TestLogMeal(t *testing.T) {
	d, meals := mealFixture(t)

	got, err := logMeal(t.Context(), d, logMealIn{
		Date:     "2026-10-13",
		At:       "19:40",
		Slot:     "夕食",
		Name:     "焼き鳥 5本",
		Qty:      "5本",
		Kcal:     iptr(420),
		ProteinG: fptr(32),
		FatG:     fptr(26),
		CarbG:    fptr(8),
	})
	if err != nil {
		t.Fatalf("記録できない: %v", err)
	}
	if got.ID == "" {
		t.Error("ID が空")
	}

	// **実際に DB に入ったかを引いて確かめる。** 戻り値だけ見ても、
	// 書き込みが起きたことの証明にならない
	from := mustDate(t, "2026-10-13")
	to := from
	items, err := meals.List(t.Context(), &from, &to)
	if err != nil {
		t.Fatalf("引けない: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("件数 = %d, want 1", len(items))
	}

	m := items[0]
	if m.Name == nil || *m.Name != "焼き鳥 5本" {
		t.Errorf("name = %v", m.Name)
	}
	// **推定由来だと後から分かるようにする。** manual と混ざると精度を評価できない
	if m.Source != openapi.MealSourceAiEstimated {
		t.Errorf("source = %v, want ai_estimated", m.Source)
	}
	if m.Kcal == nil || *m.Kcal != 420 {
		t.Errorf("kcal = %v", m.Kcal)
	}
	if m.At == nil || *m.At != "19:40" {
		t.Errorf("at = %v, want 19:40", m.At)
	}
}

func TestLogMealRejectsBadInput(t *testing.T) {
	tests := []struct {
		name string
		in   logMealIn
		want string
	}{
		{"未知の slot", logMealIn{Date: "2026-10-13", Slot: "夜食"}, "slot"},
		{"日付の形式", logMealIn{Date: "2026/10/13"}, "YYYY-MM-DD"},
		{"時刻の形式", logMealIn{Date: "2026-10-13", At: "19時40分"}, "HH:MM"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, meals := mealFixture(t)

			if _, err := logMeal(t.Context(), d, tt.in); err == nil {
				t.Fatal("エラーにならない")
			} else if !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err = %v, want %q を含む", err, tt.want)
			}

			// **弾いたときに書き込みが残っていないこと。**
			from := mustDate(t, "2026-10-13")
			to := from
			items, err := meals.List(t.Context(), &from, &to)
			if err != nil {
				t.Fatalf("引けない: %v", err)
			}
			if len(items) != 0 {
				t.Errorf("弾いたのに %d 件入っている", len(items))
			}
		})
	}
}

// TestLogMealDefaultsToToday は date 省略時に今日で入ることを見る。
func TestLogMealDefaultsToToday(t *testing.T) {
	d, _ := mealFixture(t)

	got, err := logMeal(t.Context(), d, logMealIn{Name: "プロテイン"})
	if err != nil {
		t.Fatalf("記録できない: %v", err)
	}
	if got.Date == "" {
		t.Error("date が空")
	}
}

func iptr(v int) *int { return &v }

func fptr(v float64) *float64 { return &v }

func mustDate(t *testing.T, s string) openapi_types.Date {
	t.Helper()

	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("日付が読めない: %v", err)
	}

	return openapi_types.Date{Time: d}
}
