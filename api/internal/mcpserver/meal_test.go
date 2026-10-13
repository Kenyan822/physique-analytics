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
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
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

func TestMealDateDefaultsToJSTToday(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		now  time.Time
		want string
	}{
		// **朝9時前が肝。** Truncate(24h) は絶対時刻を切るのでロケーションを見ず、
		// JST では9時に丸まる。前日になってしまう（ADR-0013）
		{"朝6時", time.Date(2028, 6, 15, 6, 0, 0, 0, timeutil.JST), "2028-06-15"},
		{"ちょうど0時", time.Date(2028, 6, 15, 0, 0, 0, 0, timeutil.JST), "2028-06-15"},
		{"夜23時", time.Date(2028, 6, 15, 23, 59, 0, 0, timeutil.JST), "2028-06-15"},
		// UTC で来ても JST の日付に直す
		{"UTCで来る", time.Date(2028, 6, 14, 21, 0, 0, 0, time.UTC), "2028-06-15"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := mealDate("", tt.now)
			if err != nil {
				t.Fatalf("mealDate: %v", err)
			}
			if day := got.Format("2006-01-02"); day != tt.want {
				t.Errorf("mealDate() = %s, want %s", day, tt.want)
			}
		})
	}
}

func TestLogMealDerivesKcalFromMacros(t *testing.T) {
	d, meals := mealFixture(t)

	// **ツールの説明が「省くと PFC から計算される」と約束している。**
	// 計算しないと、LLM への説明が嘘になる
	got, err := logMeal(t.Context(), d, logMealIn{
		Date: "2028-06-15", Name: "ゆで卵 2個",
		ProteinG: fptr(12), FatG: fptr(10), CarbG: fptr(1),
	})
	if err != nil {
		t.Fatalf("記録できない: %v", err)
	}

	list := mealsOn(t, meals, "2028-06-15")

	m := findMeal(t, list, got.ID)
	// 12×4 + 10×9 + 1×4 = 142
	if m.Kcal == nil || *m.Kcal != 142 {
		t.Errorf("Kcal = %v, want 142", m.Kcal)
	}
}

func TestLogMealKeepsGivenKcal(t *testing.T) {
	d, meals := mealFixture(t)

	// **明示された kcal を上書きしない。** 推定は kcal を直接持ってくる
	got, err := logMeal(t.Context(), d, logMealIn{
		Date: "2028-06-15", Name: "ラーメン",
		Kcal: iptr(800), ProteinG: fptr(20), FatG: fptr(20), CarbG: fptr(100),
	})
	if err != nil {
		t.Fatalf("記録できない: %v", err)
	}

	list := mealsOn(t, meals, "2028-06-15")

	m := findMeal(t, list, got.ID)
	if m.Kcal == nil || *m.Kcal != 800 {
		t.Errorf("Kcal = %v, want 800", m.Kcal)
	}
}

func TestLogMealNameOnlyHasNoKcal(t *testing.T) {
	d, meals := mealFixture(t)

	// 名前だけの記録に 0 kcal を付けない。**測っていないことが分からなくなる**
	got, err := logMeal(t.Context(), d, logMealIn{Date: "2028-06-15", Name: "外食"})
	if err != nil {
		t.Fatalf("記録できない: %v", err)
	}

	list := mealsOn(t, meals, "2028-06-15")

	if m := findMeal(t, list, got.ID); m.Kcal != nil {
		t.Errorf("Kcal = %v, want nil", *m.Kcal)
	}
}

// mealsOn はその日の記録を引き直す。**書けたことは読み直して確かめる。**
func mealsOn(t *testing.T, meals *repository.Meal, day string) []openapi.Meal {
	t.Helper()

	d := mustDate(t, day)
	list, err := meals.List(t.Context(), &d, &d)
	if err != nil {
		t.Fatalf("読み直せない: %v", err)
	}

	return list
}

func findMeal(t *testing.T, list []openapi.Meal, id string) openapi.Meal {
	t.Helper()

	for _, m := range list {
		if m.Id.String() == id {
			return m
		}
	}
	t.Fatalf("記録した行が見つからない: %s", id)

	return openapi.Meal{}
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
