// Package csvio は CSV の読み書き。
//
// スキーマは data/sample/*.csv と一致させる。**ここがずれると
// 検証基準（reference/analysis/）に食わせられなくなる**（ADR-0011）し、
// 長期バックアップの正でもある（docs/05-インフラ設計.md §7）。
package csvio

import (
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"
)

// dateLayout は CSV の日付形式。JST における日付（ADR-0013）。
const dateLayout = "2006-01-02"

// RowError は1行分の読み取り失敗。
type RowError struct {
	// Line は CSV の行番号（ヘッダが1）
	Line    int
	Message string
}

// DailyRow は daily.csv の1行。
// 欠損は nil。0 にすると「測っていない」と「0だった」が区別できなくなる。
type DailyRow struct {
	Date         time.Time
	WeightKg     *float64
	BodyfatPct   *float64
	Kcal         *int
	ProteinG     *int
	FatG         *int
	CarbG        *int
	SleepH       *float64
	Steps        *int
	Fatigue      *int
	HrvMs        *int
	RestingHr    *int
	DeepSleepMin *int
	Note         *string
}

// WorkoutRow は workouts.csv の1行。
type WorkoutRow struct {
	Date     time.Time
	Exercise string
	SetNo    int
	WeightKg float64
	Reps     int
	RIR      *int
}

// MeasureRow は measures.csv の1行。
type MeasureRow struct {
	Date         time.Time
	NeckCm       *float64
	ShoulderCm   *float64
	ChestCm      *float64
	WaistNavelCm *float64
	HipCm        *float64
	ArmRCm       *float64
	ThighRCm     *float64
	CalfRCm      *float64
}

// ヘッダの順序は data/sample/*.csv と同じにする。
var (
	dailyHeader = []string{
		"date", "weight_kg", "bodyfat_pct", "kcal", "protein_g", "fat_g", "carb_g",
		"sleep_h", "steps", "fatigue", "hrv_ms", "resting_hr", "deep_sleep_min", "note",
	}
	workoutHeader = []string{"date", "exercise", "set_no", "weight_kg", "reps", "rir"}
	measureHeader = []string{
		"date", "neck_cm", "shoulder_cm", "chest_cm", "waist_navel_cm",
		"hip_cm", "arm_r_cm", "thigh_r_cm", "calf_r_cm",
	}
)

// --- 読み取り ---

// ParseDaily は daily.csv を読む。
//
// **壊れた行は落として読めた行を返す。** 1行のミスで全部取り込めないと、
// 手で書いた過去データ（要件 I-01）が入らない。落とした行は RowError で返す。
func ParseDaily(r io.Reader) ([]DailyRow, []RowError, error) {
	return parse(r, "date", func(g getter) (DailyRow, error) {
		date, err := g.date("date")
		if err != nil {
			return DailyRow{}, err
		}

		row := DailyRow{Date: date}
		var errs []string
		set := func(err error) {
			if err != nil {
				errs = append(errs, err.Error())
			}
		}

		row.WeightKg, err = g.optFloat("weight_kg")
		set(err)
		row.BodyfatPct, err = g.optFloat("bodyfat_pct")
		set(err)
		row.Kcal, err = g.optInt("kcal")
		set(err)
		row.ProteinG, err = g.optInt("protein_g")
		set(err)
		row.FatG, err = g.optInt("fat_g")
		set(err)
		row.CarbG, err = g.optInt("carb_g")
		set(err)
		row.SleepH, err = g.optFloat("sleep_h")
		set(err)
		row.Steps, err = g.optInt("steps")
		set(err)
		row.Fatigue, err = g.optInt("fatigue")
		set(err)
		row.HrvMs, err = g.optInt("hrv_ms")
		set(err)
		row.RestingHr, err = g.optInt("resting_hr")
		set(err)
		row.DeepSleepMin, err = g.optInt("deep_sleep_min")
		set(err)
		row.Note = g.optString("note")

		if len(errs) > 0 {
			return DailyRow{}, errors.New(strings.Join(errs, " / "))
		}

		return row, nil
	})
}

// ParseWorkouts は workouts.csv を読む。
func ParseWorkouts(r io.Reader) ([]WorkoutRow, []RowError, error) {
	return parse(r, "date", func(g getter) (WorkoutRow, error) {
		date, err := g.date("date")
		if err != nil {
			return WorkoutRow{}, err
		}

		exercise := strings.TrimSpace(g.raw("exercise"))
		if exercise == "" {
			return WorkoutRow{}, errors.New("exercise が空")
		}

		setNo, err := g.reqInt("set_no")
		if err != nil {
			return WorkoutRow{}, err
		}
		weight, err := g.reqFloat("weight_kg")
		if err != nil {
			return WorkoutRow{}, err
		}
		reps, err := g.reqInt("reps")
		if err != nil {
			return WorkoutRow{}, err
		}
		rir, err := g.optInt("rir")
		if err != nil {
			return WorkoutRow{}, err
		}

		return WorkoutRow{
			Date: date, Exercise: exercise, SetNo: setNo,
			WeightKg: weight, Reps: reps, RIR: rir,
		}, nil
	})
}

// ParseMeasures は measures.csv を読む。
func ParseMeasures(r io.Reader) ([]MeasureRow, []RowError, error) {
	return parse(r, "date", func(g getter) (MeasureRow, error) {
		date, err := g.date("date")
		if err != nil {
			return MeasureRow{}, err
		}

		row := MeasureRow{Date: date}
		var errs []string
		for _, f := range []struct {
			name string
			dst  **float64
		}{
			{"neck_cm", &row.NeckCm}, {"shoulder_cm", &row.ShoulderCm},
			{"chest_cm", &row.ChestCm}, {"waist_navel_cm", &row.WaistNavelCm},
			{"hip_cm", &row.HipCm}, {"arm_r_cm", &row.ArmRCm},
			{"thigh_r_cm", &row.ThighRCm}, {"calf_r_cm", &row.CalfRCm},
		} {
			v, err := g.optFloat(f.name)
			if err != nil {
				errs = append(errs, err.Error())
				continue
			}
			*f.dst = v
		}
		if len(errs) > 0 {
			return MeasureRow{}, errors.New(strings.Join(errs, " / "))
		}

		return row, nil
	})
}

// parse はヘッダを読んで列名で引けるようにし、1行ずつ変換する。
func parse[T any](r io.Reader, required string, conv func(getter) (T, error)) ([]T, []RowError, error) {
	cr := csv.NewReader(r)
	// 列数が行によって違う CSV を許す。末尾の空欄が落ちているだけのことがある
	cr.FieldsPerRecord = -1

	header, err := cr.Read()
	if errors.Is(err, io.EOF) {
		return nil, nil, errors.New("CSV が空")
	}
	if err != nil {
		return nil, nil, fmt.Errorf("ヘッダを読めない: %w", err)
	}

	index := map[string]int{}
	for i, name := range header {
		// BOM 付きで書き出す表計算ソフトがある
		index[strings.TrimSpace(strings.TrimPrefix(name, "\ufeff"))] = i
	}
	if _, ok := index[required]; !ok {
		return nil, nil, fmt.Errorf("必須の列 %q が無い（ヘッダ: %s）", required, strings.Join(header, ","))
	}

	rows := make([]T, 0, 128)
	errs := []RowError{}

	for line := 2; ; line++ {
		rec, err := cr.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			errs = append(errs, RowError{Line: line, Message: err.Error()})
			continue
		}

		v, err := conv(getter{index: index, rec: rec})
		if err != nil {
			errs = append(errs, RowError{Line: line, Message: err.Error()})
			continue
		}
		rows = append(rows, v)
	}

	return rows, errs, nil
}

// getter は列名で値を引く。列が無い・空欄は「欠損」として扱う。
type getter struct {
	index map[string]int
	rec   []string
}

func (g getter) raw(name string) string {
	i, ok := g.index[name]
	if !ok || i >= len(g.rec) {
		return ""
	}

	return strings.TrimSpace(g.rec[i])
}

func (g getter) date(name string) (time.Time, error) {
	s := g.raw(name)
	if s == "" {
		return time.Time{}, fmt.Errorf("%s が空", name)
	}
	t, err := time.Parse(dateLayout, s)
	if err != nil {
		return time.Time{}, fmt.Errorf("%s が日付ではない: %q", name, s)
	}

	return t, nil
}

func (g getter) optFloat(name string) (*float64, error) {
	s := g.raw(name)
	if s == "" {
		return nil, nil
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("%s が数値ではない: %q", name, s)
	}

	return &v, nil
}

func (g getter) optInt(name string) (*int, error) {
	s := g.raw(name)
	if s == "" {
		return nil, nil
	}
	// "2701.0" のように小数で書かれていることがある
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return nil, fmt.Errorf("%s が数値ではない: %q", name, s)
	}
	v := int(f)

	return &v, nil
}

func (g getter) reqFloat(name string) (float64, error) {
	v, err := g.optFloat(name)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, fmt.Errorf("%s が空", name)
	}

	return *v, nil
}

func (g getter) reqInt(name string) (int, error) {
	v, err := g.optInt(name)
	if err != nil {
		return 0, err
	}
	if v == nil {
		return 0, fmt.Errorf("%s が空", name)
	}

	return *v, nil
}

func (g getter) optString(name string) *string {
	s := g.raw(name)
	if s == "" {
		return nil
	}

	return &s
}

// --- 書き出し ---

// WriteDaily は daily.csv を書く。
func WriteDaily(w io.Writer, rows []DailyRow) error {
	return write(w, dailyHeader, rows, func(r DailyRow) []string {
		return []string{
			r.Date.Format(dateLayout),
			fs(r.WeightKg), fs(r.BodyfatPct),
			is(r.Kcal), is(r.ProteinG), is(r.FatG), is(r.CarbG),
			fs(r.SleepH), is(r.Steps), is(r.Fatigue),
			is(r.HrvMs), is(r.RestingHr), is(r.DeepSleepMin),
			ss(r.Note),
		}
	})
}

// WriteWorkouts は workouts.csv を書く。
func WriteWorkouts(w io.Writer, rows []WorkoutRow) error {
	return write(w, workoutHeader, rows, func(r WorkoutRow) []string {
		return []string{
			r.Date.Format(dateLayout), r.Exercise,
			strconv.Itoa(r.SetNo),
			strconv.FormatFloat(r.WeightKg, 'f', -1, 64),
			strconv.Itoa(r.Reps),
			is(r.RIR),
		}
	})
}

// WriteMeasures は measures.csv を書く。
func WriteMeasures(w io.Writer, rows []MeasureRow) error {
	return write(w, measureHeader, rows, func(r MeasureRow) []string {
		return []string{
			r.Date.Format(dateLayout),
			fs(r.NeckCm), fs(r.ShoulderCm), fs(r.ChestCm), fs(r.WaistNavelCm),
			fs(r.HipCm), fs(r.ArmRCm), fs(r.ThighRCm), fs(r.CalfRCm),
		}
	})
}

func write[T any](w io.Writer, header []string, rows []T, conv func(T) []string) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(header); err != nil {
		return fmt.Errorf("ヘッダを書けない: %w", err)
	}
	for _, r := range rows {
		if err := cw.Write(conv(r)); err != nil {
			return fmt.Errorf("行を書けない: %w", err)
		}
	}
	cw.Flush()

	return cw.Error()
}

// 欠損は空欄で出す。0 を書くと読み戻したときに「0だった」になる。
func fs(v *float64) string {
	if v == nil {
		return ""
	}

	return strconv.FormatFloat(*v, 'f', -1, 64)
}

func is(v *int) string {
	if v == nil {
		return ""
	}

	return strconv.Itoa(*v)
}

func ss(v *string) string {
	if v == nil {
		return ""
	}

	return *v
}

// --- 値の検証 ---
//
// **DB に渡す前に Go 側で弾く。** Postgres はトランザクション内で1文でも
// 失敗すると以降のコマンドを全部拒否する（SQLSTATE 25P02）。
// 「壊れた行を飛ばして続ける」を成立させるには、制約違反を起こさないことが要る。
//
// 範囲は migrations/000003 の CHECK 制約と一致させる。

// Validate は値の範囲を確かめる。問題なければ空文字。
func (r DailyRow) Validate() string {
	checks := []struct {
		name     string
		v        *float64
		min, max float64
	}{
		{"weight_kg", r.WeightKg, 0.1, 299.9},
		{"bodyfat_pct", r.BodyfatPct, 0, 69.9},
		{"sleep_h", r.SleepH, 0, 24},
	}
	for _, c := range checks {
		if c.v != nil && (*c.v < c.min || *c.v > c.max) {
			return fmt.Sprintf("%s が範囲外: %v（%v〜%v）", c.name, *c.v, c.min, c.max)
		}
	}

	ints := []struct {
		name     string
		v        *int
		min, max int
	}{
		{"kcal", r.Kcal, 0, 1 << 30},
		{"protein_g", r.ProteinG, 0, 1 << 30},
		{"fat_g", r.FatG, 0, 1 << 30},
		{"carb_g", r.CarbG, 0, 1 << 30},
		{"steps", r.Steps, 0, 1 << 30},
		{"fatigue", r.Fatigue, 1, 5},
		{"hrv_ms", r.HrvMs, 1, 1 << 30},
		{"resting_hr", r.RestingHr, 1, 199},
		{"deep_sleep_min", r.DeepSleepMin, 0, 1 << 30},
	}
	for _, c := range ints {
		if c.v != nil && (*c.v < c.min || *c.v > c.max) {
			return fmt.Sprintf("%s が範囲外: %d（%d〜%d）", c.name, *c.v, c.min, c.max)
		}
	}

	return ""
}

// Validate は値の範囲を確かめる。問題なければ空文字。
func (r WorkoutRow) Validate() string {
	switch {
	case r.SetNo < 1:
		return fmt.Sprintf("set_no が範囲外: %d（1 以上）", r.SetNo)
	case r.WeightKg < 0 || r.WeightKg > 9999:
		return fmt.Sprintf("weight_kg が範囲外: %v", r.WeightKg)
	case r.Reps < 0:
		return fmt.Sprintf("reps が範囲外: %d（0 以上）", r.Reps)
	case r.RIR != nil && (*r.RIR < 0 || *r.RIR > 10):
		return fmt.Sprintf("rir が範囲外: %d（0〜10）", *r.RIR)
	}

	return ""
}

// Validate は値の範囲を確かめる。問題なければ空文字。
func (r MeasureRow) Validate() string {
	for _, c := range []struct {
		name string
		v    *float64
	}{
		{"neck_cm", r.NeckCm}, {"shoulder_cm", r.ShoulderCm},
		{"chest_cm", r.ChestCm}, {"waist_navel_cm", r.WaistNavelCm},
		{"hip_cm", r.HipCm}, {"arm_r_cm", r.ArmRCm},
		{"thigh_r_cm", r.ThighRCm}, {"calf_r_cm", r.CalfRCm},
	} {
		if c.v != nil && (*c.v <= 0 || *c.v > 999) {
			return fmt.Sprintf("%s が範囲外: %v", c.name, *c.v)
		}
	}

	return ""
}
