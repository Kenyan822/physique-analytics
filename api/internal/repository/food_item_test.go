package repository_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func simpleFood(name string) openapi.FoodItemInput {
	return openapi.FoodItemInput{
		Name:     name,
		Qty:      ptr("1杯"),
		ProteinG: ptr(float32(24)),
		FatG:     ptr(float32(1.5)),
		CarbG:    ptr(float32(2)),
	}
}

func TestFoodItem_引数なしで登録して引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	created, err := repo.Create(ctx, simpleFood("引数なしテスト"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Name != "引数なしテスト" {
		t.Errorf("Name = %q", created.Name)
	}
	// **基本は引数なし**（ADR-0017）
	if len(created.Components) != 0 {
		t.Errorf("Components = %v, want 空", created.Components)
	}
	if created.ProteinG == nil || *created.ProteinG != 24 {
		t.Errorf("ProteinG = %v, want 24", created.ProteinG)
	}

	// 名前で絞る。他が commit した行が見えるので全件では数えられない
	items, err := repo.List(ctx, "引数なしテスト")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) == 0 {
		t.Fatal("登録したものが一覧に出ない")
	}
}

func TestFoodItem_引数つきで登録できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	in := openapi.FoodItemInput{
		Name: "引数つきテスト",
		Components: &[]openapi.FoodItemComponent{
			{Name: "鶏ひき肉", Unit: ptr("g"), BasisAmount: 100, DefaultAmount: 200,
				ProteinG: ptr(float32(17.5)), FatG: ptr(float32(12))},
			{Name: "砂糖", Unit: ptr("g"), BasisAmount: 100, DefaultAmount: 10,
				CarbG: ptr(float32(99.2))},
		},
	}

	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if len(created.Components) != 2 {
		t.Fatalf("Components = %d件, want 2", len(created.Components))
	}
	// 表示順が保たれる
	if created.Components[0].Name != "鶏ひき肉" {
		t.Errorf("1つ目 = %q, want 鶏ひき肉", created.Components[0].Name)
	}
	if created.Components[0].BasisAmount != 100 {
		t.Errorf("BasisAmount = %v, want 100", created.Components[0].BasisAmount)
	}
}

func TestFoodItem_名前で絞れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	for _, n := range []string{"絞り込みプロテイン", "絞り込み鶏むね", "絞り込みプロテインバー"} {
		if _, err := repo.Create(ctx, simpleFood(n)); err != nil {
			t.Fatalf("Create(%s): %v", n, err)
		}
	}

	items, err := repo.List(ctx, "絞り込みプロテイン")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Errorf("List() = %d件, want 2: %v", len(items), names(items))
	}
}

func TestFoodItem_更新は構成をまるごと置き換える(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	in := openapi.FoodItemInput{
		Name: "更新テスト",
		Components: &[]openapi.FoodItemComponent{
			{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: ptr(float32(24))},
		},
	}
	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	// **差分更新にしない。** 消したつもりが残るのを避ける（plan と同じ判断）
	in.Components = &[]openapi.FoodItemComponent{
		{Name: "量", BasisAmount: 25, DefaultAmount: 25, ProteinG: ptr(float32(20))},
	}
	updated, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated.Components) != 1 || updated.Components[0].BasisAmount != 25 {
		t.Errorf("Components = %+v", updated.Components)
	}
}

func TestFoodItem_引数を消せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	in := openapi.FoodItemInput{
		Name: "引数を消すテスト",
		Components: &[]openapi.FoodItemComponent{
			{Name: "量", BasisAmount: 30, DefaultAmount: 30, ProteinG: ptr(float32(24))},
		},
	}
	created, _ := repo.Create(ctx, in)

	in.Components = &[]openapi.FoodItemComponent{}
	in.ProteinG = ptr(float32(24))
	updated, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(updated.Components) != 0 {
		t.Errorf("Components = %v, want 空", updated.Components)
	}
}

func TestFoodItem_消すと一覧から外れる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	const name = "消すテスト用"
	created, err := repo.Create(ctx, simpleFood(name))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.Delete(ctx, created.Id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	items, _ := repo.List(ctx, name)
	if len(items) != 0 {
		t.Errorf("List(%s) = %d件, want 0", name, len(items))
	}
}

func TestFoodItem_同じ名前は登録できない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	if _, err := repo.Create(ctx, simpleFood("重複テスト")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	// 同じ名前が並ぶと選ぶときに区別できない
	if _, err := repo.Create(ctx, simpleFood("重複テスト")); err == nil {
		t.Error("2回目がエラーにならない")
	}
}

func TestFoodItem_使った回数で並ぶ(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	// **テーブルが空である前提にしない。** トランザクションは自分の書き込みを
	// 巻き戻すだけで、他が commit した行は見える。名前で絞って2件だけを見る
	const tag = "並び順テスト"
	if _, err := repo.Create(ctx, simpleFood(tag+"あまり")); err != nil {
		t.Fatalf("Create: %v", err)
	}
	b, err := repo.Create(ctx, simpleFood(tag+"よく"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	for range 3 {
		if err := repo.MarkUsed(ctx, b.Id); err != nil {
			t.Fatalf("MarkUsed: %v", err)
		}
	}

	items, err := repo.List(ctx, tag)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("List(%s) = %d件, want 2", tag, len(items))
	}
	if items[0].Name != tag+"よく" {
		t.Errorf("並び順が違う: %v", names(items))
	}
}

func names(items []openapi.FoodItem) []string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, i.Name)
	}

	return out
}

func TestFoodItem_引数が無ければ空配列を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	// **null を返さない。** クライアントが map する前提で、
	// 引数なしの項目の方が多い（ADR-0017）ので、ここが既定の経路
	const name = "空配列テスト"
	created, err := repo.Create(ctx, simpleFood(name))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Components == nil {
		t.Error("Create の Components が nil")
	}

	got, err := repo.Get(ctx, created.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Components == nil {
		t.Error("Get の Components が nil")
	}

	items, _ := repo.List(ctx, name)
	if len(items) != 1 || items[0].Components == nil {
		t.Error("List の Components が nil")
	}
}

// ---- #218: 本体の比例 ----

func TestFoodItem_比例の設定を往復できる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	in := simpleFood("比例テスト")
	in.BaseAmount = f32(30)
	in.BaseUnit = strptr("g")
	in.ScalesWithAmount = boolptr(true)

	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.BaseAmount == nil || *created.BaseAmount != 30 {
		t.Errorf("BaseAmount = %v, want 30", created.BaseAmount)
	}
	if created.ScalesWithAmount == nil || !*created.ScalesWithAmount {
		t.Errorf("ScalesWithAmount = %v, want true", created.ScalesWithAmount)
	}

	// **引き直しても残る。** insert の returning だけ通って
	// select が列を落としている、を捕まえる
	got, err := repo.Get(ctx, created.Id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.BaseAmount == nil || *got.BaseAmount != 30 {
		t.Errorf("Get.BaseAmount = %v, want 30", got.BaseAmount)
	}
	if got.BaseUnit == nil || *got.BaseUnit != "g" {
		t.Errorf("Get.BaseUnit = %v, want g", got.BaseUnit)
	}
}

// **既定値のままなら既存と同じに見える**（#218 の移行の前提）
func TestFoodItem_比例を指定しなければ既定のまま(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	created, err := repo.Create(ctx, simpleFood("既定テスト"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.ScalesWithAmount == nil || *created.ScalesWithAmount {
		t.Errorf("ScalesWithAmount = %v, want false", created.ScalesWithAmount)
	}
	if created.BaseAmount != nil {
		t.Errorf("BaseAmount = %v, want nil", created.BaseAmount)
	}
}

func TestFoodItem_比例を後から外せる(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewFoodItem(testdb.Begin(t))

	in := simpleFood("比例やめるテスト")
	in.BaseAmount = f32(30)
	in.ScalesWithAmount = boolptr(true)
	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	off := simpleFood("比例やめるテスト")
	updated, err := repo.Update(ctx, created.Id, off)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if updated.ScalesWithAmount == nil || *updated.ScalesWithAmount {
		t.Errorf("ScalesWithAmount = %v, want false", updated.ScalesWithAmount)
	}
	if updated.BaseAmount != nil {
		t.Errorf("BaseAmount = %v, want nil", updated.BaseAmount)
	}
}

func f32(v float32) *float32  { return &v }
func strptr(s string) *string { return &s }
func boolptr(b bool) *bool    { return &b }
