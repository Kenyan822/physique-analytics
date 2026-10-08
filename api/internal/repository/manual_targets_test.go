package repository_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

func targetDay(t *testing.T, s string) time.Time {
	t.Helper()

	d, err := time.Parse(time.DateOnly, s)
	if err != nil {
		t.Fatalf("日付を読めない: %v", err)
	}

	return d
}

func targetEntry(t *testing.T, startsOn string, protein float32) openapi.ManualTargetEntryInput {
	t.Helper()

	return openapi.ManualTargetEntryInput{
		StartsOn: openapi_types.Date{Time: targetDay(t, startsOn)},
		ProteinG: protein, FatG: 70, CarbG: 250,
	}
}

// newManualTargets は空の履歴から始める。
//
// **履歴は全員で共有する表なので、他が commit した行が見える。**
// トランザクション内で消してから使う（Rollback で戻る）
func newManualTargets(t *testing.T) *repository.ManualTargets {
	t.Helper()

	repo := repository.NewManualTargets(testdb.Begin(t))
	if err := repo.Delete(t.Context()); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	return repo
}

func TestManualTargets_On_その日に適用される目標(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	for _, in := range []openapi.ManualTargetEntryInput{
		targetEntry(t, "2026-10-01", 150),
		targetEntry(t, "2026-10-08", 180),
	} {
		if _, err := repo.Add(ctx, in); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	tests := []struct {
		name string
		date string
		want float32 // 0 は「目標なし」
	}{
		{"最初の開始日より前は無い", "2026-09-30", 0},
		{"開始日当日から適用", "2026-10-01", 150},
		{"次の開始日の前日は前の目標", "2026-10-07", 150},
		{"次の開始日当日から新しい目標", "2026-10-08", 180},
		{"それ以降はずっと新しい目標", "2026-12-31", 180},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := repo.On(ctx, targetDay(t, tt.date))
			if err != nil {
				t.Fatalf("On: %v", err)
			}
			if tt.want == 0 {
				if got != nil {
					t.Errorf("On(%s) = %+v, want nil", tt.date, got)
				}

				return
			}
			if got == nil || got.ProteinG != tt.want {
				t.Errorf("On(%s) = %+v, want ProteinG %v", tt.date, got, tt.want)
			}
		})
	}
}

func TestManualTargets_On_履歴が無ければnil(t *testing.T) {
	t.Parallel()

	got, err := newManualTargets(t).On(t.Context(), targetDay(t, "2026-10-08"))
	if err != nil {
		t.Fatalf("On: %v", err)
	}
	if got != nil {
		t.Errorf("On() = %+v, want nil", got)
	}
}

func TestManualTargets_On_kcalは読むときに計算する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	if _, err := repo.Add(ctx, targetEntry(t, "2026-10-01", 180)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	got, err := repo.On(ctx, targetDay(t, "2026-10-01"))
	if err != nil {
		t.Fatalf("On: %v", err)
	}
	// Atwater 4/9/4
	want := 180*4 + 70*9 + 250*4
	if got == nil || got.Kcal == nil || *got.Kcal != want {
		t.Errorf("Kcal = %v, want %d", got, want)
	}
}

func TestManualTargets_Add_同じ開始日は上書き(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	first, err := repo.Add(ctx, targetEntry(t, "2026-10-08", 180))
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	second, err := repo.Add(ctx, targetEntry(t, "2026-10-08", 200))
	if err != nil {
		t.Fatalf("Add 2回目: %v", err)
	}

	// **2行目を作らず置き換える。** starts_on が unique
	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].ProteinG != 200 {
		t.Fatalf("List() = %+v, want 1件で P200", items)
	}
	if first.Id != second.Id {
		t.Errorf("上書きで id が変わった: %v → %v", first.Id, second.Id)
	}
}

func TestManualTargets_List_開始日の新しい順(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	for _, d := range []string{"2026-10-08", "2026-09-01", "2026-11-01"} {
		if _, err := repo.Add(ctx, targetEntry(t, d, 150)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	var got []string
	for _, it := range items {
		got = append(got, it.StartsOn.Format(time.DateOnly))
	}
	want := []string{"2026-11-01", "2026-10-08", "2026-09-01"}
	if len(got) != len(want) {
		t.Fatalf("List() = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("List()[%d] = %s, want %s", i, got[i], want[i])
		}
	}
}

func TestManualTargets_Add_名前を持てる(t *testing.T) {
	t.Parallel()
	repo := newManualTargets(t)

	name := "増量期"
	in := targetEntry(t, "2026-10-08", 180)
	in.Name = &name

	saved, err := repo.Add(t.Context(), in)
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if saved.Name == nil || *saved.Name != name {
		t.Errorf("Name = %v, want %q", saved.Name, name)
	}
}

func TestManualTargets_Put_今日から適用する(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	// 昨日までの目標を先に入れておく
	if _, err := repo.Add(ctx, targetEntry(t, "2026-01-01", 150)); err != nil {
		t.Fatalf("Add: %v", err)
	}

	saved, err := repo.Put(ctx, openapi.ManualTargets{ProteinG: 180, FatG: 70, CarbG: 250})
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	today := timeutil.Now().Format(time.DateOnly)
	if saved.StartsOn == nil || saved.StartsOn.Format(time.DateOnly) != today {
		t.Errorf("StartsOn = %v, want %s", saved.StartsOn, today)
	}

	// **前の目標は消えない。** 過去日はその値で評価される
	past, err := repo.On(ctx, targetDay(t, "2026-06-01"))
	if err != nil {
		t.Fatalf("On: %v", err)
	}
	if past == nil || past.ProteinG != 150 {
		t.Errorf("過去日の目標 = %+v, want P150", past)
	}

	now, err := repo.On(ctx, timeutil.Now())
	if err != nil {
		t.Fatalf("On: %v", err)
	}
	if now == nil || now.ProteinG != 180 {
		t.Errorf("今日の目標 = %+v, want P180", now)
	}
}

func TestManualTargets_Put_同じ日に2回なら1行(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	for _, p := range []float32{180, 200} {
		if _, err := repo.Put(ctx, openapi.ManualTargets{ProteinG: p, FatG: 70, CarbG: 250}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 1 || items[0].ProteinG != 200 {
		t.Errorf("List() = %+v, want 1件で P200", items)
	}
}

func TestManualTargets_DeleteEntry(t *testing.T) {
	t.Parallel()

	t.Run("1件だけ消える", func(t *testing.T) {
		ctx := t.Context()
		repo := newManualTargets(t)

		old, err := repo.Add(ctx, targetEntry(t, "2026-10-01", 150))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}
		cur, err := repo.Add(ctx, targetEntry(t, "2026-10-08", 180))
		if err != nil {
			t.Fatalf("Add: %v", err)
		}

		if err := repo.DeleteEntry(ctx, cur.Id); err != nil {
			t.Fatalf("DeleteEntry: %v", err)
		}

		// 消した期間は1つ前の履歴で評価される
		got, err := repo.On(ctx, targetDay(t, "2026-10-10"))
		if err != nil {
			t.Fatalf("On: %v", err)
		}
		if got == nil || got.ProteinG != 150 {
			t.Errorf("On() = %+v, want P150", got)
		}
		if err := repo.DeleteEntry(ctx, old.Id); err != nil {
			t.Fatalf("DeleteEntry: %v", err)
		}
		// 履歴が0件になれば自動計算へ
		if got, _ := repo.On(ctx, targetDay(t, "2026-10-10")); got != nil {
			t.Errorf("On() = %+v, want nil", got)
		}
	})

	t.Run("無い id は ErrNotFound", func(t *testing.T) {
		err := newManualTargets(t).DeleteEntry(t.Context(), uuid.New())
		if !repository.IsNotFound(err) {
			t.Errorf("DeleteEntry() = %v, want ErrNotFound", err)
		}
	})
}

func TestManualTargets_Delete_履歴を全部消す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	repo := newManualTargets(t)

	for _, d := range []string{"2026-10-01", "2026-10-08"} {
		if _, err := repo.Add(ctx, targetEntry(t, d, 150)); err != nil {
			t.Fatalf("Add: %v", err)
		}
	}
	if err := repo.Delete(ctx); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	// 無くても消せる（DELETE は冪等）
	if err := repo.Delete(ctx); err != nil {
		t.Fatalf("Delete 2回目: %v", err)
	}

	items, err := repo.List(ctx)
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(items) != 0 {
		t.Errorf("List() = %+v, want 空", items)
	}
}
