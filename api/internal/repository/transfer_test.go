package repository_test

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func day(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("日付: %v", err)
	}

	return d
}

func TestImportDaily_取り込んで書き出せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	rows := []csvio.DailyRow{
		{Date: day(t, "2031-11-01"), WeightKg: ptrF(75.1), Kcal: ptrI(2701), ProteinG: ptrI(154)},
		{Date: day(t, "2031-11-02"), WeightKg: ptrF(74.9)},
	}

	got, err := repo.ImportDaily(ctx, rows, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportDaily: %v", err)
	}
	if got.Imported != 2 || got.Skipped != 0 || len(got.Errors) != 0 {
		t.Fatalf("imported=%d skipped=%d errors=%+v", got.Imported, got.Skipped, got.Errors)
	}

	from, to := dateOf(t, "2031-11-01"), dateOf(t, "2031-11-30")
	out, err := repo.ExportDaily(ctx, &from, &to)
	if err != nil {
		t.Fatalf("ExportDaily: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("書き出し件数 = %d, want 2", len(out))
	}
	if out[0].WeightKg == nil || *out[0].WeightKg != 75.1 {
		t.Errorf("weight_kg = %v", out[0].WeightKg)
	}
	// 欠損は欠損のまま戻る
	if out[1].Kcal != nil {
		t.Errorf("kcal = %v, want nil", *out[1].Kcal)
	}
}

// 既定は skip。取り込みで既存を壊さない
func TestImportDaily_重複はskip(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	rows := []csvio.DailyRow{{Date: day(t, "2031-11-05"), WeightKg: ptrF(75.0)}}
	if _, err := repo.ImportDaily(ctx, rows, repository.OnDuplicateSkip); err != nil {
		t.Fatalf("1回目: %v", err)
	}

	rows[0].WeightKg = ptrF(99.9)
	got, err := repo.ImportDaily(ctx, rows, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("2回目: %v", err)
	}
	if got.Skipped != 1 || got.Imported != 0 {
		t.Errorf("imported=%d skipped=%d", got.Imported, got.Skipped)
	}

	d := dateOf(t, "2031-11-05")
	out, _ := repo.ExportDaily(ctx, &d, &d)
	if len(out) != 1 || *out[0].WeightKg != 75.0 {
		t.Errorf("上書きされている: %v", out)
	}
}

func TestImportDaily_overwriteなら上書きする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	rows := []csvio.DailyRow{{Date: day(t, "2031-11-06"), WeightKg: ptrF(75.0)}}
	if _, err := repo.ImportDaily(ctx, rows, repository.OnDuplicateSkip); err != nil {
		t.Fatalf("1回目: %v", err)
	}

	rows[0].WeightKg = ptrF(73.3)
	got, err := repo.ImportDaily(ctx, rows, repository.OnDuplicateOverwrite)
	if err != nil {
		t.Fatalf("2回目: %v", err)
	}
	if got.Imported != 1 {
		t.Errorf("imported = %d, want 1", got.Imported)
	}

	d := dateOf(t, "2031-11-06")
	out, _ := repo.ExportDaily(ctx, &d, &d)
	if len(out) != 1 || *out[0].WeightKg != 73.3 {
		t.Errorf("上書きされていない: %v", out)
	}
}

// 制約に反する値は行ごとエラーにして先に進む
func TestImportDaily_壊れた行は行番号つきで返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	got, err := repo.ImportDaily(ctx, []csvio.DailyRow{
		{Date: day(t, "2031-11-10"), WeightKg: ptrF(75.0)},
		{Date: day(t, "2031-11-11"), WeightKg: ptrF(999.0)}, // check 制約に反する
		{Date: day(t, "2031-11-12"), WeightKg: ptrF(74.0)},
	}, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportDaily: %v", err)
	}

	if got.Imported != 2 {
		t.Errorf("imported = %d, want 2（壊れた行以外は入る）", got.Imported)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %+v", got.Errors)
	}
	if got.Errors[0].Line != 3 {
		t.Errorf("エラー行 = %d, want 3", got.Errors[0].Line)
	}
}

func TestImportWorkouts_種目名で引き当てる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	got, err := repo.ImportWorkouts(ctx, []csvio.WorkoutRow{
		{Date: day(t, "2031-12-01"), Exercise: "ベンチプレス", SetNo: 1, WeightKg: 80, Reps: 8, RIR: ptrI(2)},
		{Date: day(t, "2031-12-01"), Exercise: "ベンチプレス", SetNo: 2, WeightKg: 80, Reps: 7, RIR: ptrI(1)},
	}, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportWorkouts: %v", err)
	}
	if got.Imported != 2 || len(got.Errors) != 0 {
		t.Fatalf("imported=%d errors=%+v", got.Imported, got.Errors)
	}

	from, to := dateOf(t, "2031-12-01"), dateOf(t, "2031-12-31")
	out, err := repo.ExportWorkouts(ctx, &from, &to)
	if err != nil {
		t.Fatalf("ExportWorkouts: %v", err)
	}
	if len(out) != 2 {
		t.Fatalf("書き出し件数 = %d, want 2", len(out))
	}
	if out[0].Exercise != "ベンチプレス" {
		t.Errorf("exercise = %q", out[0].Exercise)
	}
}

// 表記ゆれで似た種目が増えると時系列が分断されるので、勝手に作らない
func TestImportWorkouts_未知の種目はエラーにする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	got, err := repo.ImportWorkouts(ctx, []csvio.WorkoutRow{
		{Date: day(t, "2031-12-05"), Exercise: "謎のマシン", SetNo: 1, WeightKg: 50, Reps: 10},
	}, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportWorkouts: %v", err)
	}

	if got.Imported != 0 {
		t.Errorf("imported = %d, want 0", got.Imported)
	}
	if len(got.Errors) != 1 {
		t.Fatalf("errors = %+v", got.Errors)
	}
	if !contains(got.Errors[0].Message, "謎のマシン") {
		t.Errorf("どの種目か分からない: %q", got.Errors[0].Message)
	}
}

func TestImportMeasures_取り込んで書き出せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewTransfer(testdb.Begin(t))

	got, err := repo.ImportMeasures(ctx, []csvio.MeasureRow{
		{Date: day(t, "2031-12-10"), NeckCm: ptrF(39.0), WaistNavelCm: ptrF(86.0)},
	}, repository.OnDuplicateSkip)
	if err != nil {
		t.Fatalf("ImportMeasures: %v", err)
	}
	if got.Imported != 1 {
		t.Fatalf("imported = %d, errors=%+v", got.Imported, got.Errors)
	}

	d := dateOf(t, "2031-12-10")
	out, err := repo.ExportMeasures(ctx, &d, &d)
	if err != nil {
		t.Fatalf("ExportMeasures: %v", err)
	}
	if len(out) != 1 || out[0].NeckCm == nil || *out[0].NeckCm != 39.0 {
		t.Errorf("out = %+v", out)
	}
}

func ptrF(v float64) *float64 { return &v }
func ptrI(v int) *int         { return &v }

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}

	return -1
}

func dateOf(t *testing.T, s string) openapi_types.Date {
	t.Helper()

	return openapi_types.Date{Time: day(t, s)}
}
