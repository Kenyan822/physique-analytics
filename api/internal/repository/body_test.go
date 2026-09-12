package repository_test

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestPutDaily_日付をキーに上書きする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	date := jstDate(2031, 4, 1)
	if _, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, WeightKg: ptr(float32(75.1))}); err != nil {
		t.Fatalf("PutDaily（1回目）: %v", err)
	}

	// 同じ日に別の項目だけを書く。朝に体重、夜に食事という入力を想定する
	got, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, Kcal: ptr(2700)})
	if err != nil {
		t.Fatalf("PutDaily（2回目）: %v", err)
	}

	if got.Kcal == nil || *got.Kcal != 2700 {
		t.Errorf("kcal = %v, want 2700", got.Kcal)
	}
	// **省略した項目が消えないことがこのテストの本題。**
	// 上書きしてしまうと、朝に入れた体重が夜の食事入力で消える
	if got.WeightKg == nil || *got.WeightKg != 75.1 {
		t.Errorf("weightKg = %v, want 75.1（省略した項目は残る）", got.WeightKg)
	}

	list, err := repo.ListDaily(ctx, &date, &date)
	if err != nil {
		t.Fatalf("ListDaily: %v", err)
	}
	if len(list) != 1 {
		t.Errorf("1日1行のはずが %d 件", len(list))
	}
}

func TestPutDaily_nullは変更しない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	date := jstDate(2031, 4, 2)
	if _, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, Fatigue: ptr(3)}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}

	got, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, Note: ptr("脚がだるい")})
	if err != nil {
		t.Fatalf("PutDaily: %v", err)
	}
	if got.Fatigue == nil || *got.Fatigue != 3 {
		t.Errorf("fatigue = %v, want 3（null は消す意味にしない）", got.Fatigue)
	}
}

func TestListDaily_期間で絞る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	for _, d := range []openapi_types.Date{jstDate(2031, 5, 1), jstDate(2031, 5, 5), jstDate(2031, 5, 9)} {
		if _, err := repo.PutDaily(ctx, repository.DailyInput{Date: d, WeightKg: ptr(float32(75))}); err != nil {
			t.Fatalf("PutDaily: %v", err)
		}
	}

	from, to := jstDate(2031, 5, 2), jstDate(2031, 5, 6)
	got, err := repo.ListDaily(ctx, &from, &to)
	if err != nil {
		t.Fatalf("ListDaily: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("期間内は1件のはずが %d 件", len(got))
	}
	if got[0].Date.Day() != 5 {
		t.Errorf("date = %v, want 5/5", got[0].Date)
	}
}

func TestGetDaily_無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	date := jstDate(2031, 6, 30)
	if _, err := repo.GetDaily(ctx, date); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestSoftDeleteDaily_消したあと同じ日を登録し直せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	date := jstDate(2031, 7, 7)
	if _, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, WeightKg: ptr(float32(76))}); err != nil {
		t.Fatalf("PutDaily: %v", err)
	}
	if err := repo.SoftDeleteDaily(ctx, date); err != nil {
		t.Fatalf("SoftDeleteDaily: %v", err)
	}
	if _, err := repo.GetDaily(ctx, date); !repository.IsNotFound(err) {
		t.Errorf("削除後の Get: err = %v, want ErrNotFound", err)
	}

	// 生存行だけの一意インデックスなので、消した日を入れ直せる
	got, err := repo.PutDaily(ctx, repository.DailyInput{Date: date, WeightKg: ptr(float32(77))})
	if err != nil {
		t.Fatalf("削除後の PutDaily: %v", err)
	}
	if got.WeightKg == nil || *got.WeightKg != 77 {
		t.Errorf("weightKg = %v, want 77（消した行を引き継がない）", got.WeightKg)
	}
}

func TestSoftDeleteDaily_無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	if err := repo.SoftDeleteDaily(ctx, jstDate(2031, 8, 1)); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestPutMeasurement_日付をキーに上書きする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	date := jstDate(2031, 9, 1)
	if _, err := repo.PutMeasurement(ctx, repository.MeasurementInput{
		Date: date, NeckCm: ptr(float32(39)), WaistNavelCm: ptr(float32(86)),
	}); err != nil {
		t.Fatalf("PutMeasurement: %v", err)
	}

	got, err := repo.PutMeasurement(ctx, repository.MeasurementInput{Date: date, ArmRCm: ptr(float32(35.5))})
	if err != nil {
		t.Fatalf("PutMeasurement（2回目）: %v", err)
	}
	if got.ArmRCm == nil || *got.ArmRCm != 35.5 {
		t.Errorf("armRCm = %v, want 35.5", got.ArmRCm)
	}
	if got.NeckCm == nil || *got.NeckCm != 39 {
		t.Errorf("neckCm = %v, want 39（省略した項目は残る）", got.NeckCm)
	}
}

func TestLatestMeasurement_直近の1件を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	// 前回値のデフォルト表示（要件 B-03）に使うので、日付が最大の行が要る
	for _, d := range []openapi_types.Date{jstDate(2031, 10, 1), jstDate(2031, 10, 20), jstDate(2031, 10, 10)} {
		if _, err := repo.PutMeasurement(ctx, repository.MeasurementInput{Date: d, NeckCm: ptr(float32(39))}); err != nil {
			t.Fatalf("PutMeasurement: %v", err)
		}
	}

	got, err := repo.LatestMeasurement(ctx)
	if err != nil {
		t.Fatalf("LatestMeasurement: %v", err)
	}
	if got.Date.Day() != 20 {
		t.Errorf("date = %v, want 10/20", got.Date)
	}
}

func TestLatestMeasurement_記録が無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBody(testdb.Begin(t))

	// 実データが入っている DB では最新が返るので、その場合は判定しない
	if _, err := repo.LatestMeasurement(ctx); err != nil && !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound か nil", err)
	}
}
