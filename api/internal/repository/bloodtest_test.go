package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func bloodTestInput(day int) openapi.BloodTestInput {
	return openapi.BloodTestInput{
		Date:   jstDate(2034, 4, day),
		Clinic: ptr("テスト用クリニック"),
		Items: []openapi.BloodTestItem{
			{Name: "ヘモグロビン", Value: ptr(float32(15.2)), Unit: ptr("g/dL"),
				RefLow: ptr(float32(13.0)), RefHigh: ptr(float32(17.0))},
			{Name: "AST(GOT)", Value: ptr(float32(48)), Unit: ptr("U/L"),
				RefLow: ptr(float32(10)), RefHigh: ptr(float32(40))},
			{Name: "HBs抗原", TextValue: ptr("陰性")},
		},
	}
}

func TestCreateBloodTest_検査票の並びを保つ(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBloodTest(testdb.Begin(t))

	got, err := repo.Create(ctx, bloodTestInput(1))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if len(got.Items) != 3 {
		t.Fatalf("項目 = %d, want 3", len(got.Items))
	}
	// 名前で並べ替えない。検査票と見比べられなくなる
	if got.Items[0].Name != "ヘモグロビン" || got.Items[2].Name != "HBs抗原" {
		t.Errorf("並びが変わっている: %v", []string{got.Items[0].Name, got.Items[2].Name})
	}
}

func TestCreateBloodTest_基準外を判定する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBloodTest(testdb.Begin(t))

	got, err := repo.Create(ctx, bloodTestInput(2))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// ヘモグロビン 15.2（13-17）は正常、AST 48（10-40）は高い
	if got.Items[0].Flag == nil || *got.Items[0].Flag != openapi.RefNormal {
		t.Errorf("ヘモグロビン = %v, want normal", got.Items[0].Flag)
	}
	if got.Items[1].Flag == nil || *got.Items[1].Flag != openapi.RefHigh {
		t.Errorf("AST = %v, want high", got.Items[1].Flag)
	}
	// **基準が無い項目は判定しない**
	if got.Items[2].Flag == nil || *got.Items[2].Flag != openapi.RefUnknown {
		t.Errorf("HBs抗原 = %v, want unknown", got.Items[2].Flag)
	}
	if got.OutOfRangeCount == nil || *got.OutOfRangeCount != 1 {
		t.Errorf("OutOfRangeCount = %v, want 1", got.OutOfRangeCount)
	}
}

func TestCreateBloodTest_同じ日は弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBloodTest(testdb.Begin(t))

	if _, err := repo.Create(ctx, bloodTestInput(3)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 同じ日に2回採血することは無い
	if _, err := repo.Create(ctx, bloodTestInput(3)); !repository.IsConflict(err) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestUpdateBloodTest_項目を全入れ替えする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBloodTest(testdb.Begin(t))

	created, err := repo.Create(ctx, bloodTestInput(4))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	in := bloodTestInput(4)
	in.Items = []openapi.BloodTestItem{{Name: "総テストステロン", Value: ptr(float32(620)), Unit: ptr("ng/dL")}}
	got, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "総テストステロン" {
		t.Errorf("Items = %+v", got.Items)
	}
}

func TestDeleteBloodTest_消したら同じ日で作り直せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewBloodTest(testdb.Begin(t))

	created, err := repo.Create(ctx, bloodTestInput(5))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目: err = %v, want ErrNotFound", err)
	}

	if _, err := repo.Create(ctx, bloodTestInput(5)); err != nil {
		t.Errorf("削除後の Create: %v", err)
	}
}
