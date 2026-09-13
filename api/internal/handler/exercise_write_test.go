package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func postJSON(t *testing.T, srv http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	buf, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("リクエストを組み立てられない: %v", err)
	}
	req := httptest.NewRequestWithContext(t.Context(), method, path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	return rec
}

func TestCreateExercise_201と作成した種目を返す(t *testing.T) {
	t.Parallel()

	created := openapi.Exercise{Id: uuid.New(), Name: "自作種目", MuscleGroup: openapi.Chest}
	stub := &stubExercises{createReturns: created}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := postJSON(t, srv, http.MethodPost, "/v1/exercises", openapi.ExerciseInput{
		Name: "自作種目", MuscleGroup: openapi.Chest,
	})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotInput == nil {
		t.Fatal("Create が呼ばれていない")
	}
	if stub.gotInput.Name != "自作種目" || stub.gotInput.MuscleGroup != openapi.Chest {
		t.Errorf("入力が渡っていない: %+v", stub.gotInput)
	}
}

// openapi.yaml: name は minLength 1。空文字は 400
func TestCreateExercise_名前が空なら400(t *testing.T) {
	t.Parallel()

	srv := handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
	rec := postJSON(t, srv, http.MethodPost, "/v1/exercises", map[string]any{
		"name": "", "muscleGroup": "胸",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	if got := rec.Header().Get("Content-Type"); got != "application/problem+json" {
		t.Errorf("Content-Type = %q, want problem+json", got)
	}
}

func TestCreateExercise_未知の部位なら400(t *testing.T) {
	t.Parallel()

	srv := handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
	rec := postJSON(t, srv, http.MethodPost, "/v1/exercises", map[string]any{
		"name": "自作種目", "muscleGroup": "存在しない部位",
	})

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

// 名前の重複は 400（openapi.yaml が createExercise に 409 を定義していないため）
func TestCreateExercise_名前が重複したら400(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{createErr: repository.ErrConflict}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := postJSON(t, srv, http.MethodPost, "/v1/exercises", openapi.ExerciseInput{
		Name: "ベンチプレス", MuscleGroup: openapi.Chest,
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}

	var problem openapi.Problem
	if err := json.Unmarshal(rec.Body.Bytes(), &problem); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if problem.Detail == nil {
		t.Error("何が重複したかが detail に出ていない")
	}
}

func TestUpdateExercise_200を返す(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	stub := &stubExercises{updateReturns: openapi.Exercise{Id: id, Name: "更新後", MuscleGroup: openapi.Abs}}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := postJSON(t, srv, http.MethodPatch, "/v1/exercises/"+id.String(), openapi.ExerciseInput{
		Name: "更新後", MuscleGroup: openapi.Abs,
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotID != id {
		t.Errorf("id = %v, want %v", stub.gotID, id)
	}
}

func TestUpdateExercise_存在しなければ404(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{updateErr: repository.ErrNotFound}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := postJSON(t, srv, http.MethodPatch, "/v1/exercises/"+uuid.New().String(),
		openapi.ExerciseInput{Name: "無い", MuscleGroup: openapi.Abs})

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}

// 名前の重複は 409（updateExercise には 409 が定義されている）
func TestUpdateExercise_名前が重複したら409(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{updateErr: repository.ErrConflict}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := postJSON(t, srv, http.MethodPatch, "/v1/exercises/"+uuid.New().String(),
		openapi.ExerciseInput{Name: "ベンチプレス", MuscleGroup: openapi.Chest})

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestDeleteExercise_204でボディを返さない(t *testing.T) {
	t.Parallel()

	srv := handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"/v1/exercises/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
	if rec.Body.Len() != 0 {
		t.Errorf("204 なのにボディがある: %s", rec.Body.String())
	}
}

func TestDeleteExercise_存在しなければ404(t *testing.T) {
	t.Parallel()

	stub := &stubExercises{deleteErr: repository.ErrNotFound}
	srv := handler.NewRouter(handler.New(stubPinger{}, stub, &stubWorkouts{}, &stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, &stubEstimator{}, &stubPhotos{}, &stubBlobs{}))

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodDelete,
		"/v1/exercises/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (body=%s)", rec.Code, rec.Body.String())
	}
}
