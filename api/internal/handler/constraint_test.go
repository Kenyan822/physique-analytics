package handler_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// realViolation は**実際に** Postgres に制約違反を起こさせ、返ってきたエラーを返す。
// モックで PgError を組み立てると、本当に 23514 が返るかを確かめていない
func realViolation(t *testing.T, sql string, args ...any) error {
	t.Helper()

	_, err := testdb.Begin(t).Exec(t.Context(), sql, args...)
	if err == nil {
		t.Fatal("制約違反が起きていない")
	}

	return err
}

func TestResponseError_制約違反は500にしない(t *testing.T) {
	t.Parallel()

	check := realViolation(t, `insert into meals (date, source) values ('2032-05-01', 'guess')`)
	fk := realViolation(t,
		`insert into workout_sets (session_id, exercise_id, set_no, weight_kg, reps) values ($1, $2, 1, 50, 10)`,
		uuid.New(), uuid.New())
	unique := realViolation(t, `insert into manual_targets (starts_on, protein_g, fat_g, carb_g)
		select '1999-01-01', 1, 1, 1 from generate_series(1, 2)`)

	tests := []struct {
		name       string
		err        error
		want       int
		constraint string
		detail     string
	}{
		{"CHECK 違反は422", fmt.Errorf("食事を記録できない: %w", check), http.StatusUnprocessableEntity, "meals_source_check", "許されていない"},
		{"外部キー違反は422", fmt.Errorf("セットを記録できない: %w", fk), http.StatusUnprocessableEntity, "workout_sets_session_id_fkey", "存在しない"},
		{"個別に変換していない一意制約違反は409", fmt.Errorf("目標を保存できない: %w", unique), http.StatusConflict, "manual_targets_starts_on_key", "既に存在する"},
		{"制約違反でなければ500のまま", errors.New("connection refused"), http.StatusInternalServerError, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			stub := &stubMeals{createErr: tt.err}
			rec := postJSON(t, mealServer(stub), http.MethodPost, "/v1/meals",
				json.RawMessage(`{"date":"2032-05-01","kcal":100}`))

			if rec.Code != tt.want {
				t.Fatalf("status = %d, want %d, body = %s", rec.Code, tt.want, rec.Body)
			}

			var p openapi.Problem
			if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
				t.Fatalf("decode: %v", err)
			}
			detail := ""
			if p.Detail != nil {
				detail = *p.Detail
			}

			if tt.constraint != "" {
				// **どの制約か分かる。** DB を直接見ないと原因が分からなかった（#263）
				if !strings.Contains(detail, tt.constraint) || !strings.Contains(detail, tt.detail) {
					t.Errorf("detail = %q, want 制約名 %q と %q を含む", detail, tt.constraint, tt.detail)
				}
			}
			// **テーブルの内部構造・SQL・値は晒さない**
			for _, leak := range []string{"relation", "SQLSTATE", "insert into", "Failing row", "guess"} {
				if strings.Contains(rec.Body.String(), leak) {
					t.Errorf("応答に %q が含まれている: %s", leak, rec.Body)
				}
			}
			if tt.want == http.StatusInternalServerError && detail != "" {
				t.Errorf("500 の detail = %q, want 空（内部の中身を返さない）", detail)
			}
		})
	}
}

// 既存の 404 / 409 の扱いを壊さない
func TestResponseError_既存の404は変わらない(t *testing.T) {
	t.Parallel()

	stub := &stubMeals{updateErr: fmt.Errorf("食事: %w", repository.ErrNotFound)}
	rec := postJSON(t, mealServer(stub), http.MethodPatch, "/v1/meals/"+uuid.NewString(),
		json.RawMessage(`{"date":"2032-05-01"}`))

	if rec.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404, body = %s", rec.Code, rec.Body)
	}
}
