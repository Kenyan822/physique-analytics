package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func mealInput(day int, name string) repository.MealInput {
	return repository.MealInput{
		Date:     jstDate(2032, 5, day),
		Name:     &name,
		Kcal:     ptr(300),
		ProteinG: ptr(float32(25)),
	}
}

func TestCreateMeal_記録して一覧で引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	slot := openapi.Lunch
	in := mealInput(1, "サラダチキン")
	in.Slot = &slot
	in.Qty = ptr("1個")

	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name == nil || *created.Name != "サラダチキン" {
		t.Errorf("Name = %v, want サラダチキン", created.Name)
	}
	if created.Slot == nil || *created.Slot != openapi.Lunch {
		t.Errorf("Slot = %v, want 昼食", created.Slot)
	}
	// 既定は手入力。AI 推定と区別できないと分析の確度が測れない
	if created.Source != openapi.Manual {
		t.Errorf("Source = %q, want manual", created.Source)
	}

	from, to := jstDate(2032, 5, 1), jstDate(2032, 5, 1)
	list, err := repo.List(ctx, &from, &to)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("件数 = %d, want 1", len(list))
	}
}

func TestCreateMeal_ID指定は冪等(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	id := testdb.RandomUUID()
	in := mealInput(2, "プロテイン")
	in.ID = &id

	first, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// オフラインからの再送で二重に入らないこと（要件 T-07 と同じ理由）
	second, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create（再送）: %v", err)
	}
	if first.Id != second.Id {
		t.Errorf("ID が違う: %v / %v", first.Id, second.Id)
	}

	from, to := jstDate(2032, 5, 2), jstDate(2032, 5, 2)
	list, _ := repo.List(ctx, &from, &to)
	if len(list) != 1 {
		t.Errorf("件数 = %d, want 1", len(list))
	}
}

func TestUpdateMeal(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	created, err := repo.Create(ctx, mealInput(3, "白米"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	in := mealInput(3, "白米 200g")
	in.Kcal = ptr(336)
	got, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Name == nil || *got.Name != "白米 200g" || got.Kcal == nil || *got.Kcal != 336 {
		t.Errorf("更新されていない: %+v", got)
	}
}

func TestUpdateMeal_無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	if _, err := repo.Update(ctx, testdb.RandomUUID(), mealInput(4, "x")); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteMeal_論理削除する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	created, err := repo.Create(ctx, mealInput(5, "ゆで卵"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}

	from, to := jstDate(2032, 5, 5), jstDate(2032, 5, 5)
	list, _ := repo.List(ctx, &from, &to)
	if len(list) != 0 {
		t.Errorf("件数 = %d, want 0", len(list))
	}
	if err := repo.SoftDelete(ctx, created.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目の削除: err = %v, want ErrNotFound", err)
	}
}

func TestSuggestions_頻度順に返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	// **記録履歴がマスタになる**（要件 N-02）
	name := "テスト用サラダチキン" + testdb.RandomUUID().String()[:8]
	other := "テスト用ゆで卵" + testdb.RandomUUID().String()[:8]
	for day := 1; day <= 3; day++ {
		if _, err := repo.Create(ctx, mealInput(day, name)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}
	if _, err := repo.Create(ctx, mealInput(1, other)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Suggestions(ctx, "テスト用", 20)
	if err != nil {
		t.Fatalf("Suggestions: %v", err)
	}
	if len(got) < 2 {
		t.Fatalf("件数 = %d, want 2以上", len(got))
	}
	if got[0].Name != name {
		t.Errorf("先頭 = %q, want %q（回数が多い方）", got[0].Name, name)
	}
	if got[0].Count != 3 {
		t.Errorf("Count = %d, want 3", got[0].Count)
	}
	// 直近の値をそのまま初期値に使えるようにする
	if got[0].Kcal == nil || *got[0].Kcal != 300 {
		t.Errorf("Kcal = %v, want 300", got[0].Kcal)
	}
	if got[0].LastDate == nil || got[0].LastDate.Day() != 3 {
		t.Errorf("LastDate = %v, want 5/3", got[0].LastDate)
	}
}

func TestSuggestions_部分一致で絞る(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	uniq := testdb.RandomUUID().String()[:8]
	if _, err := repo.Create(ctx, mealInput(6, "焼き鮭"+uniq)); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, mealInput(6, "納豆"+uniq)); err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Suggestions(ctx, "焼き鮭"+uniq, 20)
	if err != nil {
		t.Fatalf("Suggestions: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("件数 = %d, want 1", len(got))
	}
}

func TestCopyMeals_別の日に写す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	breakfast := openapi.Breakfast
	in := mealInput(10, "オートミール")
	in.Slot = &breakfast
	if _, err := repo.Create(ctx, in); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := repo.Create(ctx, mealInput(10, "間食のナッツ")); err != nil {
		t.Fatalf("Create: %v", err)
	}

	from, to := jstDate(2032, 5, 10), jstDate(2032, 5, 11)
	got, err := repo.Copy(ctx, from, to, nil)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("複製 = %d 件, want 2", len(got))
	}
	// **新しい ID で作る。** 同じ ID にすると元の記録が移動してしまう
	if got[0].Date.Day() != 11 {
		t.Errorf("Date = %v, want 5/11", got[0].Date)
	}
}

func TestCopyMeals_slotで絞れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	breakfast, dinner := openapi.Breakfast, openapi.Dinner
	a := mealInput(20, "朝のもの")
	a.Slot = &breakfast
	b := mealInput(20, "夜のもの")
	b.Slot = &dinner
	for _, in := range []repository.MealInput{a, b} {
		if _, err := repo.Create(ctx, in); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	got, err := repo.Copy(ctx, jstDate(2032, 5, 20), jstDate(2032, 5, 21), &breakfast)
	if err != nil {
		t.Fatalf("Copy: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("複製 = %d 件, want 1", len(got))
	}
	if got[0].Name == nil || *got[0].Name != "朝のもの" {
		t.Errorf("Name = %v, want 朝のもの", got[0].Name)
	}
}

func TestCreateMeal_時刻を記録して読み出せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	in := mealInput(11, "鶏むね")
	in.At = ptr("19:40")

	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	// **秒を返さない。** DB は time 型で 19:40:00 を持つが、API は HH:MM で揃える
	if created.At == nil || *created.At != "19:40" {
		t.Fatalf("At = %v, want 19:40", created.At)
	}

	got, err := repo.Get(ctx, created.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.At == nil || *got.At != "19:40" {
		t.Errorf("Get の At = %v, want 19:40", got.At)
	}
}

func TestCreateMeal_時刻が無い記録も読める(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	// **既存の記録には時刻が無い。** null のまま読めないと過去分が全部落ちる
	created, err := repo.Create(ctx, mealInput(12, "時刻なし"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.At != nil {
		t.Errorf("At = %v, want nil", created.At)
	}
}

func TestCreateMeal_真夜中と正午(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	// 00:00 は「時刻が無い」と取り違えられやすい
	for _, at := range []string{"00:00", "12:00", "23:59"} {
		in := mealInput(13, "境界"+at)
		in.At = &at

		created, err := repo.Create(ctx, in)
		if err != nil {
			t.Fatalf("Create(%s): %v", at, err)
		}
		if created.At == nil || *created.At != at {
			t.Errorf("At = %v, want %s", created.At, at)
		}
	}
}

func TestUpdateMeal_時刻を変えられる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewMeal(testdb.Begin(t))

	in := mealInput(14, "昼")
	in.At = ptr("12:00")
	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	in.At = ptr("13:30")
	updated, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.At == nil || *updated.At != "13:30" {
		t.Errorf("At = %v, want 13:30", updated.At)
	}
}
