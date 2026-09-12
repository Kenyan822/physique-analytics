// Package csvio_test は CSV の読み書きのテスト。
//
// スキーマは data/sample/*.csv と一致させる。ここがずれると
// 検証基準（reference/analysis/）に食わせられなくなる（ADR-0011）。
package csvio_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
)

func TestParseDaily_サンプルと同じ列を読める(t *testing.T) {
	t.Parallel()

	const in = `date,weight_kg,bodyfat_pct,kcal,protein_g,fat_g,carb_g,sleep_h,steps,fatigue,hrv_ms,resting_hr,deep_sleep_min,note
2026-09-07,75.1,19.6,2701,154,78,346,7.4,6808,2,67,52,71,
2026-09-08,74.9,,2500,150,70,320,,,,,,,朝だけ計測
`

	rows, errs, err := csvio.ParseDaily(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseDaily: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("errors = %+v", errs)
	}
	if len(rows) != 2 {
		t.Fatalf("件数 = %d, want 2", len(rows))
	}

	if rows[0].Date.Format("2006-01-02") != "2026-09-07" {
		t.Errorf("date = %v", rows[0].Date)
	}
	if rows[0].WeightKg == nil || *rows[0].WeightKg != 75.1 {
		t.Errorf("weight_kg = %v", rows[0].WeightKg)
	}
	if rows[0].Kcal == nil || *rows[0].Kcal != 2701 {
		t.Errorf("kcal = %v", rows[0].Kcal)
	}

	// 空欄は nil。0 にすると「測っていない」と「0だった」が区別できなくなる
	if rows[1].BodyfatPct != nil {
		t.Errorf("空欄の bodyfat_pct = %v, want nil", *rows[1].BodyfatPct)
	}
	if rows[1].Note == nil || *rows[1].Note != "朝だけ計測" {
		t.Errorf("note = %v", rows[1].Note)
	}
}

func TestParseDaily_壊れた行は行番号つきで返す(t *testing.T) {
	t.Parallel()

	const in = `date,weight_kg,kcal
2026-09-07,75.1,2701
2026-09-08,ひゃくきろ,2500
まだ日付じゃない,70,2000
`

	rows, errs, err := csvio.ParseDaily(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseDaily: %v", err)
	}

	// 壊れた行だけ落として、読めた行は返す。全部止めると1行のミスで取り込めない
	if len(rows) != 1 {
		t.Errorf("読めた件数 = %d, want 1", len(rows))
	}
	if len(errs) != 2 {
		t.Fatalf("errors = %d 件, want 2: %+v", len(errs), errs)
	}
	// ヘッダが1行目。データの1行目は2行目
	if errs[0].Line != 3 {
		t.Errorf("1つ目のエラー行 = %d, want 3", errs[0].Line)
	}
	if !strings.Contains(errs[0].Message, "weight_kg") {
		t.Errorf("どの列か分からない: %q", errs[0].Message)
	}
}

func TestParseDaily_必須の列が無ければエラー(t *testing.T) {
	t.Parallel()

	const in = `weight_kg,kcal
75.1,2701
`
	if _, _, err := csvio.ParseDaily(strings.NewReader(in)); err == nil {
		t.Fatal("エラーを期待したが nil")
	}
}

// 列の順序が違っても読めないと、手で作った CSV を取り込めない
func TestParseDaily_列の順序が違っても読める(t *testing.T) {
	t.Parallel()

	const in = `kcal,date,weight_kg
2701,2026-09-07,75.1
`
	rows, _, err := csvio.ParseDaily(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseDaily: %v", err)
	}
	if len(rows) != 1 || rows[0].WeightKg == nil || *rows[0].WeightKg != 75.1 {
		t.Errorf("rows = %+v", rows)
	}
}

func TestParseWorkouts_読める(t *testing.T) {
	t.Parallel()

	const in = `date,exercise,set_no,weight_kg,reps,rir
2026-09-07,ベンチプレス,1,81.0,5,3
2026-09-07,ベンチプレス,2,81.0,5,
`

	rows, errs, err := csvio.ParseWorkouts(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseWorkouts: %v", err)
	}
	if len(errs) != 0 {
		t.Fatalf("errors = %+v", errs)
	}
	if len(rows) != 2 {
		t.Fatalf("件数 = %d, want 2", len(rows))
	}
	if rows[0].Exercise != "ベンチプレス" || rows[0].SetNo != 1 || rows[0].WeightKg != 81.0 {
		t.Errorf("1行目 = %+v", rows[0])
	}
	// RIR は欠損を許す。過去データには入っていないことがある
	if rows[1].RIR != nil {
		t.Errorf("空欄の rir = %v, want nil", *rows[1].RIR)
	}
}

func TestParseMeasures_読める(t *testing.T) {
	t.Parallel()

	const in = `date,neck_cm,shoulder_cm,chest_cm,waist_navel_cm,hip_cm,arm_r_cm,thigh_r_cm,calf_r_cm
2026-09-13,39.0,118.0,103.0,86.0,97.0,35.0,54.0,36.0
`

	rows, errs, err := csvio.ParseMeasures(strings.NewReader(in))
	if err != nil {
		t.Fatalf("ParseMeasures: %v", err)
	}
	if len(errs) != 0 || len(rows) != 1 {
		t.Fatalf("rows=%d errs=%+v", len(rows), errs)
	}
	if rows[0].NeckCm == nil || *rows[0].NeckCm != 39.0 {
		t.Errorf("neck_cm = %v", rows[0].NeckCm)
	}
}

// --- 書き出し ---

func TestWriteWorkouts_サンプルと同じヘッダで出る(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := csvio.WriteWorkouts(&sb, []csvio.WorkoutRow{
		{Date: mustDate(t, "2026-09-07"), Exercise: "ベンチプレス", SetNo: 1, WeightKg: 81, Reps: 5, RIR: ptrInt(3)},
		{Date: mustDate(t, "2026-09-07"), Exercise: "ベンチプレス", SetNo: 2, WeightKg: 81, Reps: 5},
	})
	if err != nil {
		t.Fatalf("WriteWorkouts: %v", err)
	}

	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	if lines[0] != "date,exercise,set_no,weight_kg,reps,rir" {
		t.Errorf("ヘッダ = %q", lines[0])
	}
	if lines[1] != "2026-09-07,ベンチプレス,1,81,5,3" {
		t.Errorf("1行目 = %q", lines[1])
	}
	// 欠損は空欄。0 と区別する
	if lines[2] != "2026-09-07,ベンチプレス,2,81,5," {
		t.Errorf("2行目 = %q", lines[2])
	}
}

// 書き出したものを読み戻せないと、バックアップとして成立しない
func TestWorkouts_書き出して読み戻せる(t *testing.T) {
	t.Parallel()

	want := []csvio.WorkoutRow{
		{Date: mustDate(t, "2026-09-07"), Exercise: "ベンチプレス", SetNo: 1, WeightKg: 81.5, Reps: 5, RIR: ptrInt(3)},
		{Date: mustDate(t, "2026-09-08"), Exercise: "スクワット, 高バー", SetNo: 1, WeightKg: 100, Reps: 5},
	}

	var sb strings.Builder
	if err := csvio.WriteWorkouts(&sb, want); err != nil {
		t.Fatalf("WriteWorkouts: %v", err)
	}

	got, errs, err := csvio.ParseWorkouts(strings.NewReader(sb.String()))
	if err != nil || len(errs) != 0 {
		t.Fatalf("読み戻せない: err=%v errs=%+v", err, errs)
	}
	if len(got) != len(want) {
		t.Fatalf("件数 = %d, want %d", len(got), len(want))
	}
	// 種目名にカンマが入っていても壊れない
	if got[1].Exercise != "スクワット, 高バー" {
		t.Errorf("exercise = %q", got[1].Exercise)
	}
	if got[0].WeightKg != 81.5 {
		t.Errorf("weight_kg = %v", got[0].WeightKg)
	}
}

func TestWriteDaily_欠損は空欄で出る(t *testing.T) {
	t.Parallel()

	var sb strings.Builder
	err := csvio.WriteDaily(&sb, []csvio.DailyRow{
		{Date: mustDate(t, "2026-09-07"), WeightKg: ptrFloat(75.1), Kcal: ptrInt(2701)},
	})
	if err != nil {
		t.Fatalf("WriteDaily: %v", err)
	}

	lines := strings.Split(strings.TrimRight(sb.String(), "\n"), "\n")
	if !strings.HasPrefix(lines[0], "date,weight_kg,bodyfat_pct,kcal,") {
		t.Errorf("ヘッダ = %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "2026-09-07,75.1,,2701,") {
		t.Errorf("1行目 = %q", lines[1])
	}
}

func mustDate(t *testing.T, s string) time.Time {
	t.Helper()
	d, err := time.Parse("2006-01-02", s)
	if err != nil {
		t.Fatalf("日付を作れない: %v", err)
	}

	return d
}

func ptrInt(v int) *int           { return &v }
func ptrFloat(v float64) *float64 { return &v }
