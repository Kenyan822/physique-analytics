package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func TestReplaceSessionExercises_全置換のステータス(t *testing.T) {
	t.Parallel()

	a, b := uuid.New(), uuid.New()
	items := []openapi.SessionExercise{
		{ExerciseId: b, ExerciseName: "スクワット", MuscleGroup: openapi.Chest, ItemOrder: 1},
		{ExerciseId: a, ExerciseName: "ベンチプレス", MuscleGroup: openapi.Chest, ItemOrder: 2},
	}

	tests := []struct {
		name       string
		body       map[string]any
		rawBody    string
		replaced   []openapi.SessionExercise
		replaceErr error
		wantStatus int
	}{
		{"並び順ごと保存できる", map[string]any{"exerciseIds": []string{b.String(), a.String()}}, "", items, nil, http.StatusOK},
		{"空配列は全部外す", map[string]any{"exerciseIds": []string{}}, "", []openapi.SessionExercise{}, nil, http.StatusOK},
		{"重複は422", map[string]any{"exerciseIds": []string{a.String(), a.String()}}, "", nil,
			fmt.Errorf("重複: %w", repository.ErrInvalid), http.StatusUnprocessableEntity},
		{"存在しない種目は422", map[string]any{"exerciseIds": []string{a.String()}}, "", nil,
			fmt.Errorf("存在しない: %w", repository.ErrInvalid), http.StatusUnprocessableEntity},
		{"セットがある種目を外すのは422", map[string]any{"exerciseIds": []string{}}, "", nil,
			fmt.Errorf("記録済み: %w", repository.ErrInvalid), http.StatusUnprocessableEntity},
		{"存在しないセッションは404", map[string]any{"exerciseIds": []string{a.String()}}, "", nil,
			fmt.Errorf("セッション: %w", repository.ErrNotFound), http.StatusNotFound},
		{"UUID でない値は400", map[string]any{"exerciseIds": []string{"not-uuid"}}, "", nil, nil, http.StatusBadRequest},
		{"exerciseIds が無ければ422", map[string]any{}, "", nil, nil, http.StatusUnprocessableEntity},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubWorkouts{replaced: tt.replaced, replaceErr: tt.replaceErr}
			sid := uuid.New()
			rec := postJSON(t, workoutServer(stub), http.MethodPut,
				"/v1/workout-sessions/"+sid.String()+"/exercises", tt.body)

			if rec.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d (body=%s)", rec.Code, tt.wantStatus, rec.Body.String())
			}
			if tt.wantStatus != http.StatusOK {
				return
			}

			if stub.gotID != sid {
				t.Errorf("セッション ID = %s, want %s", stub.gotID, sid)
			}
			var got openapi.SessionExercises
			if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
				t.Fatalf("レスポンスを読めない: %v", err)
			}
			if len(got.Items) != len(tt.replaced) {
				t.Errorf("items = %d 件, want %d", len(got.Items), len(tt.replaced))
			}
		})
	}
}

// 並び順は body の順のまま repository に渡る（itemOrder の元になる）
func TestReplaceSessionExercises_bodyの順で渡す(t *testing.T) {
	t.Parallel()

	a, b, c := uuid.New(), uuid.New(), uuid.New()
	stub := &stubWorkouts{replaced: []openapi.SessionExercise{}}
	rec := postJSON(t, workoutServer(stub), http.MethodPut,
		"/v1/workout-sessions/"+uuid.NewString()+"/exercises",
		map[string]any{"exerciseIds": []string{c.String(), a.String(), b.String()}})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}
	want := []uuid.UUID{c, a, b}
	if len(stub.gotExerciseIDs) != 3 {
		t.Fatalf("渡った ID = %v, want %v", stub.gotExerciseIDs, want)
	}
	for i := range want {
		if stub.gotExerciseIDs[i] != want[i] {
			t.Errorf("gotExerciseIDs[%d] = %s, want %s", i, stub.gotExerciseIDs[i], want[i])
		}
	}
}

// 422 は errors に原因を載せる（クライアントが理由を出せる）
func TestReplaceSessionExercises_422は理由を返す(t *testing.T) {
	t.Parallel()

	stub := &stubWorkouts{replaceErr: fmt.Errorf("セットを記録済みの種目は外せない: %w", repository.ErrInvalid)}
	rec := postJSON(t, workoutServer(stub), http.MethodPut,
		"/v1/workout-sessions/"+uuid.NewString()+"/exercises",
		map[string]any{"exerciseIds": []string{}})

	var p openapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("problem を読めない: %v", err)
	}
	if p.Errors == nil || len(*p.Errors) == 0 || (*p.Errors)[0].Field != "exerciseIds" {
		t.Errorf("errors = %+v, want field=exerciseIds", p.Errors)
	}
}

// GET に exercises が含まれる。行が無ければ空配列（null ではない）
func TestGetWorkoutSession_exercisesを含む(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   []openapi.SessionExercise
		want int
	}{
		{"並びがある", []openapi.SessionExercise{{ExerciseId: uuid.New(), ExerciseName: "x", MuscleGroup: openapi.Chest, ItemOrder: 1}}, 1},
		{"行が0件なら空配列", []openapi.SessionExercise{}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubWorkouts{session: openapi.WorkoutSession{Id: uuid.New(), Sets: []openapi.WorkoutSet{}, Exercises: tt.in}}
			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/workout-sessions/"+stub.session.Id.String(), nil)
			rec := httptest.NewRecorder()
			workoutServer(stub).ServeHTTP(rec, req)

			var raw map[string]json.RawMessage
			if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
				t.Fatalf("読めない: %v", err)
			}
			ex, ok := raw["exercises"]
			if !ok || string(ex) == "null" {
				t.Fatalf("exercises = %s, want 配列", ex)
			}
			var items []openapi.SessionExercise
			if err := json.Unmarshal(ex, &items); err != nil {
				t.Fatal(err)
			}
			if len(items) != tt.want {
				t.Errorf("exercises = %d 件, want %d", len(items), tt.want)
			}
		})
	}
}
