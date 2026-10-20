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

type stubDays struct {
	records repository.DayRecords
	calls   int
	gotDate time.Time
}

func (s *stubDays) Records(_ context.Context, date time.Time) (repository.DayRecords, error) {
	s.calls++
	s.gotDate = date

	return s.records, nil
}

func dayServer(days handler.DayRepository, streaks handler.StreakRepository) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{},
		&stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{})
	if days != nil {
		srv = srv.WithDays(days)
	}
	if streaks != nil {
		srv = srv.WithStreaks(streaks)
	}

	return handler.NewRouter(srv)
}

// getDay は生の JSON も返す。null と省略を区別したいので
func getDay(t *testing.T, srv http.Handler, date string) (int, openapi.DayDetail, map[string]json.RawMessage) {
	t.Helper()

	rec := postJSON(t, srv, http.MethodGet, "/v1/days/"+date, nil)

	var detail openapi.DayDetail
	var raw map[string]json.RawMessage
	if rec.Code == http.StatusOK {
		if err := json.Unmarshal(rec.Body.Bytes(), &detail); err != nil {
			t.Fatalf("decode: %v, body = %s", err, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("decode raw: %v", err)
		}
	}

	return rec.Code, detail, raw
}

func f64(v float64) *float64 { return &v }

// 値はすべて架空
func TestGetDay_食事と筋トレと体組成を1回で返す(t *testing.T) {
	t.Parallel()

	tname, order := "胸", 1
	st := &stubDays{records: repository.DayRecords{
		Kcal:     1800,
		Consumed: analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200},
		Targets:  []analytics.StreakTarget{{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}}},
		Workout: &repository.DayWorkout{TemplateName: &tname, DayOrder: &order, Exercises: []repository.DayExercise{
			{Name: "種目A", SetCount: 5, TopWeightKg: 60},
			{Name: "種目B", SetCount: 3, TopWeightKg: 20},
		}},
		Body: &repository.DayBody{WeightKg: f64(70.5), BodyFatPct: f64(15)},
	}}

	code, got, _ := getDay(t, dayServer(st, nil), "2026-10-03")

	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	if st.calls != 1 || st.gotDate.Format(time.DateOnly) != "2026-10-03" {
		t.Errorf("Records は日付つきで1回: calls=%d date=%v", st.calls, st.gotDate)
	}
	if got.Date.Format(time.DateOnly) != "2026-10-03" {
		t.Errorf("date = %v", got.Date)
	}

	m := got.Meals
	if m.Consumed.Kcal != 1800 || m.Consumed.ProteinG != 150 || m.Consumed.FatG != 60 || m.Consumed.CarbG != 200 {
		t.Errorf("consumed = %+v", m.Consumed)
	}
	// 目標の kcal は PFC から出す（4/9/4）: 180*4 + 70*9 + 250*4 = 2350
	if m.Target == nil || m.Target.ProteinG != 180 || m.Target.Kcal != 2350 {
		t.Errorf("target = %+v", m.Target)
	}

	w := got.Workout
	if w == nil || w.SetCount != 8 || len(w.Exercises) != 2 {
		t.Fatalf("workout = %+v, want 8 セット・2 種目", w)
	}
	if w.TemplateName == nil || *w.TemplateName != "胸" || w.DayOrder == nil || *w.DayOrder != 1 {
		t.Errorf("template = %v, dayOrder = %v", w.TemplateName, w.DayOrder)
	}
	if e := w.Exercises[0]; e.ExerciseName != "種目A" || e.SetCount != 5 || e.TopWeightKg != 60 {
		t.Errorf("exercises[0] = %+v", e)
	}

	b := got.Body
	if b == nil || b.WeightKg == nil || *b.WeightKg != 70.5 || b.BodyFatPct == nil || *b.BodyFatPct != 15 {
		t.Errorf("body = %+v", b)
	}
}

// 未達のとき、どの栄養素がどれだけ足りないかが shortfall から分かる
func TestGetDay_未達はshortfallで理由が分かる(t *testing.T) {
	t.Parallel()

	st := &stubDays{records: repository.DayRecords{
		Consumed: analytics.PFC{ProteinG: 150, FatG: 70, CarbG: 250},
		Targets:  []analytics.StreakTarget{{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}}},
	}}

	_, got, _ := getDay(t, dayServer(st, nil), "2026-10-03")

	if got.Meals.GoalMet == nil || *got.Meals.GoalMet {
		t.Errorf("goalMet = %v, want false", got.Meals.GoalMet)
	}
	s := got.Meals.Shortfall
	if s == nil || s.ProteinG != -30 || s.FatG != 0 || s.CarbG != 0 {
		t.Errorf("shortfall = %+v, want P -30 / F 0 / C 0（負なら足りない）", s)
	}
}

// 目標が引けない日は goalMet も target も shortfall も null（false ではない）
func TestGetDay_目標が引けない日はnull(t *testing.T) {
	t.Parallel()

	st := &stubDays{records: repository.DayRecords{
		Consumed: analytics.PFC{ProteinG: 100},
		// 最初の目標より前の日
		Targets: []analytics.StreakTarget{{StartsOn: "2026-10-05", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}}},
	}}

	code, _, raw := getDay(t, dayServer(st, nil), "2026-10-03")

	if code != http.StatusOK {
		t.Fatalf("status = %d", code)
	}
	var meals map[string]json.RawMessage
	if err := json.Unmarshal(raw["meals"], &meals); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"goalMet", "target", "shortfall"} {
		if v, ok := meals[k]; !ok || string(v) != "null" {
			t.Errorf("meals.%s = %s (present=%v), want null", k, v, ok)
		}
	}
}

// 記録が無い日も 200。consumed は 0、workout と body は null
func TestGetDay_記録が無い日も200(t *testing.T) {
	t.Parallel()

	for name, srv := range map[string]http.Handler{
		"元データが空":         dayServer(&stubDays{}, nil),
		"repository 未設定": dayServer(nil, nil),
	} {
		code, got, raw := getDay(t, srv, "2026-10-03")

		if code != http.StatusOK {
			t.Fatalf("%s: status = %d", name, code)
		}
		if got.Meals.Consumed != (openapi.Macros{}) {
			t.Errorf("%s: consumed = %+v, want 0", name, got.Meals.Consumed)
		}
		for _, k := range []string{"workout", "body"} {
			if v, ok := raw[k]; !ok || string(v) != "null" {
				t.Errorf("%s: %s = %s (present=%v), want null", name, k, v, ok)
			}
		}
	}
}

// 過去日は当時の目標で判定される
func TestGetDay_過去日は当時の目標で判定する(t *testing.T) {
	t.Parallel()

	targets := []analytics.StreakTarget{
		{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}},
		{StartsOn: "2026-10-05", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}},
	}
	consumed := analytics.PFC{ProteinG: 150, FatG: 60, CarbG: 200}

	tests := []struct {
		date       string
		wantMet    bool
		wantTarget float64
	}{
		{"2026-10-02", true, 150},
		{"2026-10-05", false, 180},
	}
	for _, tt := range tests {
		st := &stubDays{records: repository.DayRecords{Consumed: consumed, Targets: targets}}
		_, got, _ := getDay(t, dayServer(st, nil), tt.date)

		if got.Meals.GoalMet == nil || *got.Meals.GoalMet != tt.wantMet {
			t.Errorf("%s: goalMet = %v, want %v", tt.date, got.Meals.GoalMet, tt.wantMet)
		}
		if got.Meals.Target == nil || got.Meals.Target.ProteinG != float32(tt.wantTarget) {
			t.Errorf("%s: target = %+v, want P%v", tt.date, got.Meals.Target, tt.wantTarget)
		}
	}
}

// 達成判定は /v1/streaks と一致する（境界・目標なし・目標 0 を含む）
func TestGetDay_達成判定はstreaksと一致する(t *testing.T) {
	t.Parallel()

	targets := []analytics.StreakTarget{
		{StartsOn: "2026-10-01", PFC: analytics.PFC{ProteinG: 180, FatG: 70, CarbG: 250}},
		{StartsOn: "2026-10-10"},
	}
	consumed := []analytics.PFC{
		{ProteinG: 198, FatG: 77, CarbG: 275},   // ちょうど +10%
		{ProteinG: 162, FatG: 63, CarbG: 225},   // ちょうど -10%
		{ProteinG: 161.9, FatG: 63, CarbG: 225}, // わずかに外
		{},
		{ProteinG: 1},
	}

	for _, date := range []string{"2026-09-30", "2026-10-03", "2026-10-10"} {
		for _, c := range consumed {
			day := &stubDays{records: repository.DayRecords{Consumed: c, Targets: targets}}
			streak := &stubStreaks{inputs: repository.StreakInputs{
				Targets: targets, Consumed: map[string]analytics.PFC{date: c},
			}}
			srv := dayServer(day, streak)

			_, got, _ := getDay(t, srv, date)
			_, items := getStreaks(t, srv, "from="+date+"&to="+date)

			gotMet := "null"
			if got.Meals.GoalMet != nil {
				gotMet = map[bool]string{true: "true", false: "false"}[*got.Meals.GoalMet]
			}
			if want := metOf(items[0]); gotMet != want {
				t.Errorf("%s %+v: days = %s, streaks = %s", date, c, gotMet, want)
			}
		}
	}
}

func TestGetDay_不正な日付は400(t *testing.T) {
	t.Parallel()

	for _, d := range []string{"yesterday", "2026-13-45"} {
		if code, _, _ := getDay(t, dayServer(&stubDays{}, nil), d); code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400", d, code)
		}
	}
}
