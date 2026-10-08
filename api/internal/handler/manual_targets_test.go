package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// stubManualTargets は手動目標の履歴の差し替え。
type stubManualTargets struct {
	// byDate は日付ごとに適用される目標。履歴の引き方そのものは repository のテストが見る
	byDate  map[string]*openapi.ManualTargets
	current *openapi.ManualTargets
	asked   []string
	got     *openapi.ManualTargets
	deleted bool

	entries        []openapi.ManualTargetEntry
	added          *openapi.ManualTargetEntryInput
	deletedEntryID uuid.UUID
}

func (s *stubManualTargets) On(_ context.Context, date time.Time) (*openapi.ManualTargets, error) {
	key := date.Format(time.DateOnly)
	s.asked = append(s.asked, key)
	if t, ok := s.byDate[key]; ok {
		return t, nil
	}

	return s.current, nil
}

func (s *stubManualTargets) Put(_ context.Context, in openapi.ManualTargets) (openapi.ManualTargets, error) {
	s.got = &in
	s.current = &in

	return in, nil
}

func (s *stubManualTargets) Delete(context.Context) error {
	s.deleted = true
	s.current = nil

	return nil
}

func (s *stubManualTargets) List(context.Context) ([]openapi.ManualTargetEntry, error) {
	return s.entries, nil
}

func (s *stubManualTargets) Add(_ context.Context, in openapi.ManualTargetEntryInput) (openapi.ManualTargetEntry, error) {
	s.added = &in

	return openapi.ManualTargetEntry{
		Id: uuid.New(), StartsOn: in.StartsOn, ProteinG: in.ProteinG, FatG: in.FatG, CarbG: in.CarbG,
	}, nil
}

func (s *stubManualTargets) DeleteEntry(_ context.Context, id uuid.UUID) error {
	s.deletedEntryID = id
	for _, e := range s.entries {
		if e.Id == id {
			return nil
		}
	}

	return repository.ErrNotFound
}

func manualServer(mt *stubManualTargets, p *stubPlan, se *stubSeries, m *stubMeals) http.Handler {
	srv := handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, m, p, se,
		&stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{})

	return handler.NewRouter(srv.WithManualTargets(mt))
}

func TestGetManualTargets_未設定ならnull(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/manual", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var body struct {
		Targets *openapi.ManualTargets `json:"targets"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body.Targets != nil {
		t.Errorf("targets = %+v, want null", body.Targets)
	}
}

func TestPutManualTargets_保存する(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{}
	rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodPut, "/v1/targets/manual",
		json.RawMessage(`{"proteinG":180,"fatG":70,"carbG":250}`))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if stub.got == nil || stub.got.ProteinG != 180 {
		t.Errorf("保存されていない: %+v", stub.got)
	}
}

func TestPutManualTargets_範囲外は422(t *testing.T) {
	t.Parallel()

	for _, body := range []string{
		`{"proteinG":-1,"fatG":70,"carbG":250}`,
		`{"proteinG":180,"fatG":70,"carbG":9999}`,
	} {
		rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
			http.MethodPut, "/v1/targets/manual", json.RawMessage(body))

		if rec.Code != http.StatusUnprocessableEntity {
			t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
		}
	}
}

func TestDeleteManualTargets_消せる(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{current: &openapi.ManualTargets{ProteinG: 180}}
	rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodDelete, "/v1/targets/manual", nil)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body)
	}
	if !stub.deleted {
		t.Error("消していない")
	}
}

func TestGetDailyTargets_手動目標があれば優先する(t *testing.T) {
	t.Parallel()

	// **フェーズが無くても目標が出る。** いままで 422 で何も出なかった
	mt := &stubManualTargets{current: &openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}}
	rec := postJSON(t, manualServer(mt, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/2026-11-01", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Target == nil || got.Target.ProteinG != 180 {
		t.Fatalf("Target = %+v", got.Target)
	}
	// **どちらの値かを返す。** 体重が動いても目標が変わらない理由が分かるように
	if got.TargetSource == nil || *got.TargetSource != openapi.DailyTargetsTargetSourceManual {
		t.Errorf("TargetSource = %v, want manual", got.TargetSource)
	}
	if got.Remaining == nil || got.Remaining.ProteinG != 180 {
		t.Errorf("Remaining = %+v", got.Remaining)
	}
}

func TestGetDailyTargets_手動が無ければ自動計算(t *testing.T) {
	t.Parallel()

	p := &stubPlan{plan: planWithPhase()}
	se := &stubSeries{points: series(21, "2026-11-01", 75, 2400)}
	rec := postJSON(t, manualServer(&stubManualTargets{}, p, se, &stubMeals{}),
		http.MethodGet, "/v1/targets/2026-11-01", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.TargetSource == nil || *got.TargetSource != openapi.DailyTargetsTargetSourceComputed {
		t.Errorf("TargetSource = %v, want computed", got.TargetSource)
	}
}

// **過去日はその日の目標で残量を出す。** 現在の目標で計算すると、
// 目標を変えた日より前の達成が未達に見える（#241）
func TestGetDailyTargets_過去日は当時の目標で出す(t *testing.T) {
	t.Parallel()

	mt := &stubManualTargets{
		current: &openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250},
		byDate: map[string]*openapi.ManualTargets{
			"2026-10-01": {ProteinG: 150, FatG: 70, CarbG: 250},
		},
	}
	rec := postJSON(t, manualServer(mt, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/2026-10-01", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.DailyTargets
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Target == nil || got.Target.ProteinG != 150 {
		t.Errorf("Target = %+v, want P150（当時の目標）", got.Target)
	}
	if len(mt.asked) == 0 || mt.asked[0] != "2026-10-01" {
		t.Errorf("引いた日 = %v, want 2026-10-01", mt.asked)
	}
}

func TestListManualTargetEntries_履歴を返す(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{entries: []openapi.ManualTargetEntry{
		{Id: uuid.New(), StartsOn: mustDate2("2026-10-08"), ProteinG: 180, FatG: 70, CarbG: 250, Kcal: 2350},
		{Id: uuid.New(), StartsOn: mustDate2("2026-10-01"), ProteinG: 150, FatG: 70, CarbG: 250, Kcal: 2230},
	}}
	rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/manual/entries", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var body struct {
		Items []openapi.ManualTargetEntry `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Items) != 2 || body.Items[0].ProteinG != 180 {
		t.Errorf("items = %+v", body.Items)
	}
}

func TestListManualTargetEntries_空なら空配列(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
		http.MethodGet, "/v1/targets/manual/entries", nil)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	// **null ではなく []。** クライアントが items.map で落ちないように
	if got := rec.Body.String(); !strings.Contains(got, `"items":[]`) {
		t.Errorf("body = %s, want items:[]", got)
	}
}

func TestCreateManualTargetEntry(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		body string
		want int
	}{
		{"追加できる", `{"startsOn":"2026-10-08","proteinG":180,"fatG":70,"carbG":250}`, http.StatusCreated},
		{"未来の日付は予約になる", `{"startsOn":"2027-01-01","proteinG":180,"fatG":70,"carbG":250}`, http.StatusCreated},
		{"範囲外は422", `{"startsOn":"2026-10-08","proteinG":-1,"fatG":70,"carbG":250}`, http.StatusUnprocessableEntity},
		{"名前が長すぎると422", `{"name":"` + strings.Repeat("あ", 101) + `","startsOn":"2026-10-08","proteinG":180,"fatG":70,"carbG":250}`, http.StatusUnprocessableEntity},
		{"名前が空文字だと422", `{"name":"","startsOn":"2026-10-08","proteinG":180,"fatG":70,"carbG":250}`, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubManualTargets{}
			rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
				http.MethodPost, "/v1/targets/manual/entries", json.RawMessage(tt.body))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body)
			}
			if tt.want == http.StatusCreated && stub.added == nil {
				t.Error("追加されていない")
			}
			if tt.want != http.StatusCreated && stub.added != nil {
				t.Error("422 なのに追加された")
			}
		})
	}
}

func TestDeleteManualTargetEntry(t *testing.T) {
	t.Parallel()

	id := uuid.New()

	t.Run("消せる", func(t *testing.T) {
		t.Parallel()

		stub := &stubManualTargets{entries: []openapi.ManualTargetEntry{{Id: id}}}
		rec := postJSON(t, manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{}),
			http.MethodDelete, "/v1/targets/manual/entries/"+id.String(), nil)

		if rec.Code != http.StatusNoContent {
			t.Fatalf("status = %d, want 204, body = %s", rec.Code, rec.Body)
		}
		if stub.deletedEntryID != id {
			t.Errorf("消した id = %v, want %v", stub.deletedEntryID, id)
		}
	})

	t.Run("無い id は404", func(t *testing.T) {
		t.Parallel()

		rec := postJSON(t, manualServer(&stubManualTargets{}, &stubPlan{}, &stubSeries{}, &stubMeals{}),
			http.MethodDelete, "/v1/targets/manual/entries/"+id.String(), nil)

		if rec.Code != http.StatusNotFound {
			t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
		}
	})
}

// **既存クライアント（iOS・Web）が叩く3本は、パスもレスポンスの形も変えない。**
// 履歴に作り替えても、ここが変わると iOS が壊れる
func TestManualTargets_既存の3本の形は変わらない(t *testing.T) {
	t.Parallel()

	stub := &stubManualTargets{current: &openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250}}
	srv := manualServer(stub, &stubPlan{}, &stubSeries{}, &stubMeals{})

	t.Run("GET は targets に1件", func(t *testing.T) {
		rec := postJSON(t, srv, http.MethodGet, "/v1/targets/manual", nil)
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}

		var body map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if _, ok := body["targets"]; !ok || len(body) != 1 {
			t.Errorf("body = %s, want {targets: …} のみ", rec.Body)
		}

		var targets map[string]any
		if err := json.Unmarshal(body["targets"], &targets); err != nil {
			t.Fatalf("decode targets: %v", err)
		}
		for _, k := range []string{"proteinG", "fatG", "carbG"} {
			if _, ok := targets[k]; !ok {
				t.Errorf("targets に %s が無い: %v", k, targets)
			}
		}
	})

	t.Run("PUT は ManualTargets を返す", func(t *testing.T) {
		rec := postJSON(t, srv, http.MethodPut, "/v1/targets/manual",
			json.RawMessage(`{"proteinG":200,"fatG":60,"carbG":200}`))
		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
		}

		var got openapi.ManualTargets
		if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
			t.Fatalf("decode: %v", err)
		}
		if got.ProteinG != 200 {
			t.Errorf("ProteinG = %v, want 200", got.ProteinG)
		}
	})

	t.Run("DELETE は204", func(t *testing.T) {
		rec := postJSON(t, srv, http.MethodDelete, "/v1/targets/manual", nil)
		if rec.Code != http.StatusNoContent {
			t.Errorf("status = %d, want 204, body = %s", rec.Code, rec.Body)
		}
		if !stub.deleted {
			t.Error("消していない")
		}
	})
}
