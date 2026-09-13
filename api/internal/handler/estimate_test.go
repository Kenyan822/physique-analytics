package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/vision"
)

// stubEstimator は実 API を叩かない。**テストで課金を発生させない。**
type stubEstimator struct {
	enabled bool
	got     *vision.Request
	result  vision.Estimate
	err     error
}

func (s *stubEstimator) Enabled() bool { return s.enabled }

func (s *stubEstimator) Estimate(_ context.Context, req vision.Request) (vision.Estimate, error) {
	s.got = &req
	return s.result, s.err
}

func estimateServer(e *stubEstimator) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, &stubTransfer{}, &stubBody{}, &stubMeals{},
		&stubPlan{}, &stubSeries{}, &stubMealSets{}, &stubContests{}, &stubBlood{}, e, &stubPhotos{}, &stubBlobs{}))
}

func imageForm(t *testing.T, note string) (*bytes.Buffer, string) {
	t.Helper()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)

	h := make(map[string][]string)
	h["Content-Disposition"] = []string{`form-data; name="image"; filename="a.jpg"`}
	h["Content-Type"] = []string{"image/jpeg"}
	part, err := w.CreatePart(h)
	if err != nil {
		t.Fatalf("パートを作れない: %v", err)
	}
	if _, err := part.Write([]byte{0xFF, 0xD8, 0xFF}); err != nil {
		t.Fatalf("画像を書けない: %v", err)
	}

	if note != "" {
		if err := w.WriteField("note", note); err != nil {
			t.Fatalf("補足を書けない: %v", err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatalf("フォームを閉じられない: %v", err)
	}

	return &buf, w.FormDataContentType()
}

func postForm(t *testing.T, srv http.Handler, body *bytes.Buffer, contentType string) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/meals/estimate", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	return rec
}

func TestEstimateMeal_下書きとして返す(t *testing.T) {
	t.Parallel()

	kcal := 620
	p, f, c := 48.0, 6.0, 88.0
	stub := &stubEstimator{enabled: true, result: vision.Estimate{
		Name: "鶏むねと白米", Kcal: &kcal, ProteinG: &p, FatG: &f, CarbG: &c,
		Confidence: "medium", Note: "食器のサイズから量を推定",
	}}

	body, ct := imageForm(t, "鶏むね200g、白米150g")
	rec := postForm(t, estimateServer(stub), body, ct)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body)
	}

	var got openapi.MealEstimate
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatalf("JSON: %v", err)
	}
	if got.Name != "鶏むねと白米" || got.Kcal == nil || *got.Kcal != 620 {
		t.Errorf("推定が返っていない: %+v", got)
	}
	// **推定値であることをデータに残す**（docs/02-データモデル.md）
	if got.Source != openapi.AiEstimated {
		t.Errorf("Source = %q, want ai_estimated", got.Source)
	}
	// 量を添えると精度が上がるので、補足は必ず渡す
	if stub.got == nil || stub.got.Note != "鶏むね200g、白米150g" {
		t.Errorf("補足が渡っていない: %+v", stub.got)
	}
	if stub.got.MimeType != "image/jpeg" {
		t.Errorf("MimeType = %q", stub.got.MimeType)
	}
}

func TestEstimateMeal_未設定なら503(t *testing.T) {
	t.Parallel()

	body, ct := imageForm(t, "")
	rec := postForm(t, estimateServer(&stubEstimator{enabled: false}), body, ct)

	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503, body = %s", rec.Code, rec.Body)
	}
	// **直し方と代替手段を返す**
	if !jsonContains(rec.Body.String(), "ANTHROPIC_API_KEY") ||
		!jsonContains(rec.Body.String(), "手で入力") {
		t.Errorf("body = %s, want 設定方法と代替手段", rec.Body)
	}
}

func TestEstimateMeal_推定に失敗したら503(t *testing.T) {
	t.Parallel()

	stub := &stubEstimator{enabled: true, err: errors.New("推定 API が 429 を返した")}
	body, ct := imageForm(t, "")
	rec := postForm(t, estimateServer(stub), body, ct)

	// リクエストが悪いわけではないので 4xx にしない。
	// 利用者は「手で入力する」に切り替えればよい
	if rec.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503, body = %s", rec.Code, rec.Body)
	}
}

func TestEstimateMeal_画像が無ければ422(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	_ = w.WriteField("note", "鶏むね")
	if err := w.Close(); err != nil {
		t.Fatalf("フォームを閉じられない: %v", err)
	}

	rec := postForm(t, estimateServer(&stubEstimator{enabled: true}), &buf, w.FormDataContentType())
	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422, body = %s", rec.Code, rec.Body)
	}
}
