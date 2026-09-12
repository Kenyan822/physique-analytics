package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

type stubTransfer struct {
	daily    []csvio.DailyRow
	workouts []csvio.WorkoutRow
	measures []csvio.MeasureRow

	gotOnDuplicate repository.OnDuplicate
	gotDailyRows   int
	result         repository.ImportResult
}

func (s *stubTransfer) ExportDaily(context.Context, *openapi_types.Date, *openapi_types.Date) ([]csvio.DailyRow, error) {
	return s.daily, nil
}

func (s *stubTransfer) ExportWorkouts(context.Context, *openapi_types.Date, *openapi_types.Date) ([]csvio.WorkoutRow, error) {
	return s.workouts, nil
}

func (s *stubTransfer) ExportMeasures(context.Context, *openapi_types.Date, *openapi_types.Date) ([]csvio.MeasureRow, error) {
	return s.measures, nil
}

func (s *stubTransfer) ImportDaily(_ context.Context, rows []csvio.DailyRow, on repository.OnDuplicate) (repository.ImportResult, error) {
	s.gotDailyRows = len(rows)
	s.gotOnDuplicate = on
	return s.result, nil
}

func (s *stubTransfer) ImportWorkouts(_ context.Context, rows []csvio.WorkoutRow, on repository.OnDuplicate) (repository.ImportResult, error) {
	s.gotOnDuplicate = on
	return repository.ImportResult{Imported: len(rows)}, nil
}

func (s *stubTransfer) ImportMeasures(_ context.Context, rows []csvio.MeasureRow, on repository.OnDuplicate) (repository.ImportResult, error) {
	s.gotOnDuplicate = on
	return repository.ImportResult{Imported: len(rows)}, nil
}

func transferServer(tr *stubTransfer) http.Handler {
	return handler.NewRouter(handler.New(stubPinger{}, &stubExercises{}, &stubWorkouts{},
		&stubTemplates{}, &stubSync{}, tr, &stubBody{}, &stubMeals{}))
}

func TestExportCsv_workoutsをCSVで返す(t *testing.T) {
	t.Parallel()

	tr := &stubTransfer{workouts: []csvio.WorkoutRow{
		{Date: mustDay(t, "2026-09-07"), Exercise: "ベンチプレス", SetNo: 1, WeightKg: 81, Reps: 5, RIR: ptrInt(3)},
	}}

	rec := httptest.NewRecorder()
	transferServer(tr).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/export/csv?resource=workouts", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}
	if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/csv") {
		t.Errorf("Content-Type = %q, want text/csv", ct)
	}

	lines := strings.Split(strings.TrimRight(rec.Body.String(), "\n"), "\n")
	if lines[0] != "date,exercise,set_no,weight_kg,reps,rir" {
		t.Errorf("ヘッダ = %q", lines[0])
	}
	if lines[1] != "2026-09-07,ベンチプレス,1,81,5,3" {
		t.Errorf("1行目 = %q", lines[1])
	}
}

func TestExportCsv_resourceが不正なら400(t *testing.T) {
	t.Parallel()

	rec := httptest.NewRecorder()
	transferServer(&stubTransfer{}).ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(),
		http.MethodGet, "/v1/export/csv?resource=nope", nil))

	if rec.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
}

func uploadCSV(t *testing.T, srv http.Handler, resource, onDup, body string) *httptest.ResponseRecorder {
	t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if err := mw.WriteField("resource", resource); err != nil {
		t.Fatalf("resource: %v", err)
	}
	if onDup != "" {
		if err := mw.WriteField("onDuplicate", onDup); err != nil {
			t.Fatalf("onDuplicate: %v", err)
		}
	}
	fw, err := mw.CreateFormFile("file", resource+".csv")
	if err != nil {
		t.Fatalf("file: %v", err)
	}
	if _, err := fw.Write([]byte(body)); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	req := httptest.NewRequestWithContext(t.Context(), http.MethodPost, "/v1/import/csv", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())

	rec := httptest.NewRecorder()
	srv.ServeHTTP(rec, req)

	return rec
}

func TestImportCsv_取り込み件数を返す(t *testing.T) {
	t.Parallel()

	tr := &stubTransfer{result: repository.ImportResult{Imported: 2, Skipped: 0}}
	rec := uploadCSV(t, transferServer(tr), "daily", "", `date,weight_kg
2026-09-07,75.1
2026-09-08,74.9
`)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 (body=%s)", rec.Code, rec.Body.String())
	}

	var body openapi.ImportCsv200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v", err)
	}
	if body.Imported != 2 {
		t.Errorf("imported = %d, want 2", body.Imported)
	}
	if tr.gotDailyRows != 2 {
		t.Errorf("渡った行数 = %d, want 2", tr.gotDailyRows)
	}
	// 既定は skip。取り込みで既存を壊さない
	if tr.gotOnDuplicate != repository.OnDuplicateSkip {
		t.Errorf("onDuplicate = %q, want skip", tr.gotOnDuplicate)
	}
}

func TestImportCsv_onDuplicateが渡る(t *testing.T) {
	t.Parallel()

	tr := &stubTransfer{}
	rec := uploadCSV(t, transferServer(tr), "daily", "overwrite", "date,weight_kg\n2026-09-07,75.1\n")

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d (body=%s)", rec.Code, rec.Body.String())
	}
	if tr.gotOnDuplicate != repository.OnDuplicateOverwrite {
		t.Errorf("onDuplicate = %q, want overwrite", tr.gotOnDuplicate)
	}
}

// 1行のミスで全部止めない。読めた行は取り込み、落とした行を返す
func TestImportCsv_壊れた行を行番号つきで返す(t *testing.T) {
	t.Parallel()

	tr := &stubTransfer{result: repository.ImportResult{Imported: 1}}
	rec := uploadCSV(t, transferServer(tr), "daily", "", `date,weight_kg
2026-09-07,75.1
2026-09-08,ひゃくきろ
`)

	var body openapi.ImportCsv200JSONResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("JSON として読めない: %v (body=%s)", err, rec.Body.String())
	}
	if body.Imported != 1 {
		t.Errorf("imported = %d, want 1", body.Imported)
	}
	if len(body.Errors) != 1 {
		t.Fatalf("errors = %+v", body.Errors)
	}
	if body.Errors[0].Line != 3 {
		t.Errorf("エラー行 = %d, want 3", body.Errors[0].Line)
	}
}

func TestImportCsv_必須の列が無ければ422(t *testing.T) {
	t.Parallel()

	rec := uploadCSV(t, transferServer(&stubTransfer{}), "daily", "", "weight_kg\n75.1\n")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func TestImportCsv_未知のresourceは422(t *testing.T) {
	t.Parallel()

	rec := uploadCSV(t, transferServer(&stubTransfer{}), "nope", "", "date\n2026-09-07\n")

	if rec.Code != http.StatusUnprocessableEntity {
		t.Errorf("status = %d, want 422 (body=%s)", rec.Code, rec.Body.String())
	}
}

func mustDay(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("日付: %v", err)
	}

	return d
}
