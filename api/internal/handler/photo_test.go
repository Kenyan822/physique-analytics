package handler_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubPhotos struct {
	rows      []repository.PhotoRow
	guideRows []repository.PhotoRow
	gotInput  *repository.PhotoInput
	gotID     uuid.UUID
	gotBefore openapi_types.Date
	created   repository.PhotoRow
	createErr error
	deleteErr error
}

func (s *stubPhotos) List(context.Context, *openapi_types.Date, *openapi_types.Date) ([]repository.PhotoRow, error) {
	return s.rows, nil
}

func (s *stubPhotos) Guide(_ context.Context, before openapi_types.Date) ([]repository.PhotoRow, error) {
	s.gotBefore = before
	return s.guideRows, nil
}

func (s *stubPhotos) Create(_ context.Context, in repository.PhotoInput) (repository.PhotoRow, error) {
	s.gotInput = &in
	return s.created, s.createErr
}

func (s *stubPhotos) SoftDelete(_ context.Context, id uuid.UUID) error {
	s.gotID = id
	return s.deleteErr
}

// stubBlobs は署名を作らず、キーが渡っているかだけ見る。
type stubBlobs struct {
	enabled bool
	gotKeys []string
}

func (s *stubBlobs) Enabled() bool { return s.enabled }

func (s *stubBlobs) PresignGet(key string, _ time.Duration, _ time.Time) (string, error) {
	s.gotKeys = append(s.gotKeys, key)
	return "https://example.invalid/" + key + "?sig=get", nil
}

func (s *stubBlobs) PresignPut(key string, _ time.Duration, _ time.Time) (string, error) {
	s.gotKeys = append(s.gotKeys, key)
	return "https://example.invalid/" + key + "?sig=put", nil
}

func photoServer(p *stubPhotos, b *stubBlobs) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{},
		&stubEstimator{}, p, b))
}

func photoRow(pose openapi.PhotoPose, key string) repository.PhotoRow {
	return repository.PhotoRow{
		Photo:      openapi.BodyPhoto{Id: uuid.New(), Pose: pose, MimeType: "image/jpeg", ByteSize: 4000000},
		StorageKey: key,
	}
}

func TestListPhotos_署名付きURLを添える(t *testing.T) {
	t.Parallel()

	photos := &stubPhotos{rows: []repository.PhotoRow{photoRow(openapi.PoseFront, "photos/a/front.jpg")}}
	rec := httptest.NewRecorder()
	photoServer(photos, &stubBlobs{enabled: true}).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/photos", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got struct {
		Items []openapi.BodyPhoto `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Url == nil {
		t.Fatalf("URL が付いていない: %+v", got.Items)
	}
	// **公開URLではない。** 署名が付いている（ADR-0008）
	if !jsonContains(*got.Items[0].Url, "sig=get") {
		t.Errorf("URL = %q", *got.Items[0].Url)
	}
}

func TestCreatePhotoUpload_アップロード先を返す(t *testing.T) {
	t.Parallel()

	photos := &stubPhotos{created: photoRow(openapi.PoseFront, "photos/2035-03-01/front.jpg")}
	rec := postJSON(t, photoServer(photos, &stubBlobs{enabled: true}), http.MethodPost, "/v1/photos",
		map[string]any{"date": "2035-03-01", "pose": "front", "mimeType": "image/jpeg", "byteSize": 4000000})

	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.PhotoUpload
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	// **画像は API を経由させない**（ADR-0008）
	if !jsonContains(got.UploadUrl, "sig=put") {
		t.Errorf("UploadUrl = %q", got.UploadUrl)
	}
	// キーは日付で並ぶ形
	if photos.gotInput == nil || photos.gotInput.StorageKey != "photos/2035-03-01/front.jpg" {
		t.Errorf("StorageKey = %+v", photos.gotInput)
	}
}

func TestCreatePhotoUpload_同じ日の同じ向きは422(t *testing.T) {
	t.Parallel()

	photos := &stubPhotos{createErr: repository.ErrConflict}
	rec := postJSON(t, photoServer(photos, &stubBlobs{enabled: true}), http.MethodPost, "/v1/photos",
		map[string]any{"date": "2035-03-01", "pose": "front", "mimeType": "image/jpeg", "byteSize": 4000000})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Fatalf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
	// 直し方を返す
	if !jsonContains(rec.Body.String(), "先に消す") {
		t.Errorf("body = %s", rec.Body)
	}
}

func TestCreatePhotoUpload_大きすぎる画像は422(t *testing.T) {
	t.Parallel()

	rec := postJSON(t, photoServer(&stubPhotos{}, &stubBlobs{enabled: true}), http.MethodPost, "/v1/photos",
		map[string]any{"date": "2035-03-01", "pose": "front", "mimeType": "image/jpeg", "byteSize": 50000000})

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}

func TestPhotos_保存先が未設定なら503(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	photoServer(&stubPhotos{}, &stubBlobs{enabled: false}).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/v1/photos", nil))

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body)
	}
	if !jsonContains(rec.Body.String(), "R2_ACCOUNT_ID") {
		t.Errorf("body = %s, want 設定方法", rec.Body)
	}
}

func TestGetPhotoGuide_前回写真を返す(t *testing.T) {
	t.Parallel()

	photos := &stubPhotos{guideRows: []repository.PhotoRow{
		photoRow(openapi.PoseFront, "photos/2035-02-01/front.jpg"),
	}}
	rec := httptest.NewRecorder()
	photoServer(photos, &stubBlobs{enabled: true}).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodGet,
			"/v1/photos/guide?before=2035-03-01", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}
	if photos.gotBefore.Format("2006-01-02") != "2035-03-01" {
		t.Errorf("before = %v", photos.gotBefore)
	}
}

func TestDeletePhoto_204(t *testing.T) {
	t.Parallel()

	id := uuid.New()
	photos := &stubPhotos{}
	rec := httptest.NewRecorder()
	photoServer(photos, &stubBlobs{enabled: true}).ServeHTTP(rec,
		httptest.NewRequestWithContext(t.Context(), http.MethodDelete, "/v1/photos/"+id.String(), nil))

	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rec.Code)
	}
	if photos.gotID != id {
		t.Errorf("ID = %v", photos.gotID)
	}
}
