package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func setInput(name string) openapi.MealSetInput {
	slot := openapi.Breakfast

	return openapi.MealSetInput{
		Name: name,
		Slot: &slot,
		Items: []openapi.MealSetItem{
			{Name: "オートミール", Qty: ptr("80g"), Kcal: ptr(304), ProteinG: ptr(float32(11))},
			{Name: "プロテイン", Kcal: ptr(120), ProteinG: ptr(float32(24))},
		},
	}
}

func uniqueName(prefix string) string {
	return prefix + testdb.RandomUUID().String()[:8]
}

func TestCreateMealSet_項目つきで作れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	got, err := repo.Create(ctx, setInput(uniqueName("朝食セット")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(got.Items) != 2 {
		t.Fatalf("Items = %d 件, want 2", len(got.Items))
	}
	// 入力の順序を保つ。並びが変わると「いつもの朝食」に見えなくなる
	if got.Items[0].Name != "オートミール" {
		t.Errorf("先頭 = %q, want オートミール", got.Items[0].Name)
	}
	if got.Slot == nil || *got.Slot != openapi.Breakfast {
		t.Errorf("Slot = %v, want 朝食", got.Slot)
	}
}

func TestCreateMealSet_同じ名前は弾く(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	name := uniqueName("重複テスト")
	if _, err := repo.Create(ctx, setInput(name)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 同じ名前が並ぶと選ぶときに区別できない
	if _, err := repo.Create(ctx, setInput(name)); !repository.IsConflict(err) {
		t.Errorf("err = %v, want ErrConflict", err)
	}
}

func TestUpdateMealSet_項目を全入れ替えする(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	created, err := repo.Create(ctx, setInput(uniqueName("更新テスト")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	in := setInput(created.Name)
	in.Items = []openapi.MealSetItem{{Name: "ゆで卵", Kcal: ptr(80)}}
	got, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(got.Items) != 1 || got.Items[0].Name != "ゆで卵" {
		t.Errorf("Items = %+v, want ゆで卵 1件", got.Items)
	}
}

func TestDeleteMealSet_論理削除する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	created, err := repo.Create(ctx, setInput(uniqueName("削除テスト")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目: err = %v, want ErrNotFound", err)
	}

	// 消したら同じ名前で作り直せる（生存行だけの一意インデックス）
	if _, err := repo.Create(ctx, setInput(created.Name)); err != nil {
		t.Errorf("削除後の Create: %v", err)
	}
}

func TestApplyMealSet_その日の記録に展開する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	repo := repository.NewMealSet(tx)
	meals := repository.NewMeal(tx)

	created, err := repo.Create(ctx, setInput(uniqueName("展開テスト")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	date := jstDate(2033, 3, 1)
	got, err := repo.Apply(ctx, created.Id, date, nil)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("展開 = %d 件, want 2", len(got))
	}
	// セットの既定の区分が入る
	if got[0].Slot == nil || *got[0].Slot != openapi.Breakfast {
		t.Errorf("Slot = %v, want 朝食", got[0].Slot)
	}

	// **meals に写る。** 写した後は個別に編集・削除できる
	list, err := meals.List(ctx, &date, &date)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 2 {
		t.Errorf("meals = %d 件, want 2", len(list))
	}
}

func TestApplyMealSet_区分を上書きできる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	created, err := repo.Create(ctx, setInput(uniqueName("区分テスト")))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	dinner := openapi.Dinner
	got, err := repo.Apply(ctx, created.Id, jstDate(2033, 3, 2), &dinner)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if got[0].Slot == nil || *got[0].Slot != openapi.Dinner {
		t.Errorf("Slot = %v, want 夕食", got[0].Slot)
	}
}

func TestApplyMealSet_無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMealSet(testdb.Begin(t))

	if _, err := repo.Apply(ctx, testdb.RandomUUID(), jstDate(2033, 3, 3), nil); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}
