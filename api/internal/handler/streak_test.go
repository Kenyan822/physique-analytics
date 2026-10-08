package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// stubStreaks は達成判定の元データの差し替え。
type stubStreaks struct {
	inputs repository.StreakInputs
	calls  int
	from   time.Time
	to     time.Time
}

func (s *stubStreaks) Inputs(_ context.Context, from, to time.Time) (repository.StreakInputs, error) {
	s.calls++
	s.from, s.to = from, to

	return s.inputs, nil
}

func streakServer(st handler.StreakRepository) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{},
		&stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{})

	return handler.NewRouter(srv.WithStreaks(st))
}

func getStreaks(t *testing.T, srv http.Handler, query string) (int, []openapi.StreakDay) {
	t.Helper()

	rec := postJSON(t, srv, http.MethodGet, "/v1/streaks?"+query, nil)

	var body struct {
		Items []openapi.StreakDay `json:"items"`
	}
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v, body = %s", err, rec.Body)
		}
	}

	return rec.Code, body.Items
}

func metOf(d openapi.StreakDay) string {
	if d.MealGoalMet == nil {
		return "null"
	}
	if *d.MealGoalMet {
		return "true"
	}

	return "false"
}

// 値はすべて架空
func TestGetStreaks_達成判定(t *testing.T) {
	t.Parallel()

	targets := []analytics.StreakTarget{
		{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}},
		{StartsOn: "2026-10-05", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}},
	}

	tests := []struct {
		name     string
		date     string
		consumed analytics.PFC
		want     string
	}{
		{"P・F・C すべて ±10% 以内なら true", "2026-10-06", analytics.PFC{ProteinG: 185, FatG: 72, CarbG: 240}, "true"},
		{"1つでも外れたら false", "2026-10-06", analytics.PFC{ProteinG: 185, FatG: 60, CarbG: 240}, "false"},
		{"目標が引けない日は null（false ではない）", "2026-09-30", analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}, "null"},
		{"過去日は当時の目標で判定する（旧目標なら達成）", "2026-10-02", analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}, "true"},
		{"同じ食事でも新しい目標の日は未達", "2026-10-05", analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}, "false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &stubStreaks{inputs: repository.StreakInputs{
				Targets:  targets,
				Consumed: map[string]analytics.PFC{tt.date: tt.consumed},
			}}
			code, items := getStreaks(t, streakServer(st), "from="+tt.date+"&to="+tt.date)

			if code != http.StatusOK {
				t.Fatalf("status = %d", code)
			}
			if len(items) != 1 || metOf(items[0]) != tt.want {
				t.Errorf("items = %+v, want mealGoalMet=%s", items, tt.want)
			}
		})
	}
}

func TestGetStreaks_記録が無い日も行を返す(t *testing.T) {
	t.Parallel()

	st := &stubStreaks{inputs: repository.StreakInputs{
		Trained: map[string]bool{"2026-10-02": true},
	}}
	code, items := getStreaks(t, streakServer(st), "from=2026-10-01&to=2026-10-03")

	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if len(items) != 3 {
		t.Fatalf("len = %d, want 3: %+v", len(items), items)
	}
	for i, wantTrained := range []bool{false, true, false} {
		if items[i].Trained != wantTrained {
			t.Errorf("items[%d].Trained = %v, want %v", i, items[i].Trained, wantTrained)
		}
	}
	if items[0].Date.Format(time.DateOnly) != "2026-10-01" {
		t.Errorf("昇順になっていない: %+v", items)
	}
}

func TestGetStreaks_期間は1回の問い合わせで引く(t *testing.T) {
	t.Parallel()

	st := &stubStreaks{}
	// 両端を含めて366日（2026 年は 365 日なので 2027-01-01 まで）
	if code, items := getStreaks(t, streakServer(st), "from=2026-01-01&to=2027-01-01"); code != http.StatusOK || len(items) != 366 {
		t.Fatalf("status = %d, len = %d, want 200 / 366", code, len(items))
	}
	// **1日ごとに引かない。** 366日でも元データの問い合わせは1回
	if st.calls != 1 {
		t.Errorf("Inputs の呼び出し = %d, want 1", st.calls)
	}
}

func TestGetStreaks_範囲の検証(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		query string
		want  int
	}{
		{"366日は通る", "from=2026-01-01&to=2027-01-01", http.StatusOK},
		{"367日は422", "from=2026-01-01&to=2027-01-02", http.StatusUnprocessableEntity},
		{"from が to より後は422", "from=2026-10-02&to=2026-10-01", http.StatusUnprocessableEntity},
		{"from が無いと400", "to=2026-10-01", http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			st := &stubStreaks{}
			code, _ := getStreaks(t, streakServer(st), tt.query)
			if code != tt.want {
				t.Errorf("status = %d, want %d", code, tt.want)
			}
			if tt.want != http.StatusOK && st.calls != 0 {
				t.Error("422/400 なのに DB を引いた")
			}
		})
	}
}
