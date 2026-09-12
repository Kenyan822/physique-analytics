package repository_test

import (
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func contestInput(y, month, day int, category string) openapi.ContestInput {
	return openapi.ContestInput{
		HeldOn:      jstDate(y, time.Month(month), day),
		Category:    category,
		TargetBfPct: 11.0,
		Goal:        ptr("完走・経験"),
	}
}

func TestCreateContest_登録して一覧で引ける(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	in := contestInput(2034, 5, 31, "テスト用サマスタ")

	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if created.Category != "テスト用サマスタ" {
		t.Errorf("Category = %q", created.Category)
	}
	if created.TargetBfPct != 11.0 {
		t.Errorf("TargetBfPct = %v, want 11", created.TargetBfPct)
	}

	list, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) == 0 {
		t.Fatal("一覧が空")
	}
	// 開催日の昇順。次の大会を先頭から探せるようにする
	for i := 1; i < len(list); i++ {
		if list[i].HeldOn.Before(list[i-1].HeldOn.Time) {
			t.Fatalf("昇順でない: %v → %v", list[i-1].HeldOn, list[i].HeldOn)
		}
	}
}

func TestNextContest_基準日以降で一番近いものを返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	for _, d := range []struct {
		y, m, day int
		name      string
	}{
		{2035, 5, 31, "テスト用1回目"},
		{2036, 6, 30, "テスト用2回目"},
	} {
		if _, err := repo.Create(ctx, contestInput(d.y, d.m, d.day, d.name)); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	got, err := repo.Next(ctx, jstDate(2035, 1, 1))
	if err != nil {
		t.Fatalf("Next: %v", err)
	}
	if got.Category != "テスト用1回目" {
		t.Errorf("Category = %q, want テスト用1回目", got.Category)
	}

	// **当日も「次の大会」に含める。** 当日にカウントダウンが消えると不自然
	same, err := repo.Next(ctx, jstDate(2035, 5, 31))
	if err != nil {
		t.Fatalf("Next（当日）: %v", err)
	}
	if same.Category != "テスト用1回目" {
		t.Errorf("当日: Category = %q, want テスト用1回目", same.Category)
	}
}

func TestNextContest_全部過ぎていればErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	if _, err := repo.Next(ctx, jstDate(2099, 1, 1)); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestUpdateContest(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	in := contestInput(2037, 5, 31, "テスト用更新前")
	created, err := repo.Create(ctx, in)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	in.Category = "テスト用更新後"
	in.TargetBfPct = 9.5
	got, err := repo.Update(ctx, created.Id, in)
	if err != nil {
		t.Fatalf("Update: %v", err)
	}
	if got.Category != "テスト用更新後" || got.TargetBfPct != 9.5 {
		t.Errorf("更新されていない: %+v", got)
	}
}

func TestUpdateContest_無ければErrNotFound(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	if _, err := repo.Update(ctx, testdb.RandomUUID(), contestInput(2038, 5, 31, "x")); !repository.IsNotFound(err) {
		t.Errorf("err = %v, want ErrNotFound", err)
	}
}

func TestDeleteContest_論理削除する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := repository.NewContest(testdb.Begin(t))

	created, err := repo.Create(ctx, contestInput(2039, 5, 31, "テスト用削除"))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); err != nil {
		t.Fatalf("SoftDelete: %v", err)
	}
	if err := repo.SoftDelete(ctx, created.Id); !repository.IsNotFound(err) {
		t.Errorf("2回目: err = %v, want ErrNotFound", err)
	}
}
