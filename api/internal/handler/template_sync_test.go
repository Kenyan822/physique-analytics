package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubTemplates struct {
	items    []openapi.Template
	template openapi.Template
	gotInput *repository.TemplateInput

	getErr    error
	createErr error
	updateErr error
	deleteErr error
}

func (s *stubTemplates) List(context.Context) ([]openapi.Template, error) {
	return s.items, nil
}

func (s *stubTemplates) Get(context.Context, uuid.UUID) (openapi.Template, error) {
	return s.template, s.getErr
}

func (s *stubTemplates) Create(_ context.Context, in repository.TemplateInput) (openapi.Template, error) {
	s.gotInput = &in
	return s.template, s.createErr
}

func (s *stubTemplates) Update(_ context.Context, _ uuid.UUID, in repository.TemplateInput) (openapi.Template, error) {
	s.gotInput = &in
	return s.template, s.updateErr
}

func (s *stubTemplates) SoftDelete(context.Context, uuid.UUID) error { return s.deleteErr }

type stubSync struct {
	pull     repository.PullResult
	push     repository.PushResult
	gotSince time.Time
	gotPush  *repository.PushInput
}

func (s *stubSync) Pull(_ context.Context, since time.Time) (repository.PullResult, error) {
	s.gotSince = since
	return s.pull, nil
}

func (s *stubSync) Push(_ context.Context, in repository.PushInput) (repository.PushResult, error) {
	s.gotPush = &in
	return s.push, nil
}

func fullServer(tpl *stubTemplates, sy *stubSync) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{}, tpl, sy, &stubTransfer{}, &stubBody{}, &stubMeals{}, &stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}))
}

func validTemplateBody() map[string]any {
	return map[string]any{
		"name": "Day1 胸",
		"items": []map[string]any{
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 4, "targetRepsMin": 6, "targetRepsMax": 8, "targetRir": 2},
		},
	}
}

func TestCreateTemplate_201(t *testing.T) {
	t.Parallel()

	stub := &stubTemplates{template: openapi.Template{Id: uuid.New(), Name: "Day1 胸"}}
	rec := postJSON(t, fullServer(stub, &stubSync{}), http.MethodPost, "/v1/templates", validTemplateBody())

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotInput == nil || len(stub.gotInput.Items) != 1 {
		t.Errorf("items が渡っていない: %+v", stub.gotInput)
	}
}

func TestCreateTemplate_入力の検証(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body map[string]any
	}{
		{"名前が空", map[string]any{"name": "", "items": []map[string]any{
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 4}}}},
		{"項目が空", map[string]any{"name": "空", "items": []map[string]any{}}},
		{"order が重複", map[string]any{"name": "重複", "items": []map[string]any{
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 4},
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 3}}}},
		{"targetSets が範囲外", map[string]any{"name": "範囲外", "items": []map[string]any{
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 30}}}},
		{"レップ範囲が逆", map[string]any{"name": "逆", "items": []map[string]any{
			{"exerciseId": uuid.New().String(), "order": 1, "targetSets": 4,
				"targetRepsMin": 12, "targetRepsMax": 6}}}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rec := postJSON(t, fullServer(&stubTemplates{}, &stubSync{}), http.MethodPost, "/v1/templates", tc.body)
			if rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
			}
		})
	}
}

func TestGetTemplate_存在しなければ404(t *testing.T) {
	t.Parallel()

	stub := &stubTemplates{getErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	fullServer(stub, &stubSync{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/templates/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rec.Code)
	}
}

// 消えている状態は達成されているので冪等に 204
func TestDeleteTemplate_存在しなくても204(t *testing.T) {
	t.Parallel()

	stub := &stubTemplates{deleteErr: repository.ErrNotFound}
	rec := httptest.NewRecorder()
	fullServer(stub, &stubSync{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodDelete, "/v1/templates/"+uuid.New().String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Errorf("status = %d, want 204 (body=%s)", rec.Code, rec.Body.String())
	}
}

// --- sync ---

func TestPullSync_updatedSinceが渡る(t *testing.T) {
	t.Parallel()

	since := time.Date(2026, 9, 12, 3, 0, 0, 0, time.UTC)
	stub := &stubSync{pull: repository.PullResult{ServerTime: time.Now()}}

	rec := httptest.NewRecorder()
	fullServer(&stubTemplates{}, stub).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/sync?updatedSince="+since.Format(time.RFC3339), nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if !stub.gotSince.Equal(since) {
		t.Errorf("since = %v, want %v", stub.gotSince, since)
	}
}

// updatedSince は必須。無いと全件返してしまう
func TestPullSync_updatedSinceが無ければ400(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	fullServer(&stubTemplates{}, &stubSync{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/sync", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestPushSync_競合を返す(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	now := time.Now()
	stub := &stubSync{push: repository.PushResult{
		Applied:    2,
		Conflicts:  []repository.Conflict{{Resource: "session", ID: id, ServerUpdatedAt: now}},
		ServerTime: now,
	}}

	rec := postJSON(t, fullServer(&stubTemplates{}, stub), http.MethodPost, "/v1/sync", map[string]any{
		"sessions": []map[string]any{{"id": uuid.New().String(), "date": "2026-09-12"}},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var body openapi.PushSync200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.Applied != 2 {
		t.Errorf("applied = %d, want 2", body.Applied)
	}
	if len(body.Conflicts) != 1 {
		t.Fatalf("conflicts = %d 件, want 1", len(body.Conflicts))
	}
	if body.Conflicts[0].Resource != openapi.ConflictSession {
		t.Errorf("resource = %q", body.Conflicts[0].Resource)
	}
	if body.Conflicts[0].Id != id {
		t.Errorf("id = %v, want %v", body.Conflicts[0].Id, id)
	}
}

func TestPushSync_セッションとセットが渡る(t *testing.T) {
	t.Parallel()

	stub := &stubSync{}
	sid := uuid.New()
	rec := postJSON(t, fullServer(&stubTemplates{}, stub), http.MethodPost, "/v1/sync", map[string]any{
		"sessions": []map[string]any{{"id": uuid.New().String(), "date": "2026-09-12", "note": "同期"}},
		"sets": []map[string]any{{
			"id": uuid.New().String(), "sessionId": sid.String(),
			"exerciseId": uuid.New().String(), "setNo": 1, "weightKg": 80, "reps": 8,
		}},
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if stub.gotPush == nil {
		t.Fatal("Push が呼ばれていない")
	}
	if len(stub.gotPush.Sessions) != 1 || len(stub.gotPush.Sets) != 1 {
		t.Fatalf("sessions=%d sets=%d", len(stub.gotPush.Sessions), len(stub.gotPush.Sets))
	}
	// セットの同期には sessionId が要る
	if stub.gotPush.Sets[0].SessionID == nil || *stub.gotPush.Sets[0].SessionID != sid {
		t.Errorf("sessionId = %v, want %v", stub.gotPush.Sets[0].SessionID, sid)
	}
}
