package repository_test

import (
	"testing"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

func TestPull_updatedSince以降の変更を返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)

	s, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 7, 1),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8, RIR: ptr(2)}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// **基準にはサーバ側の時刻を使う。** Go の time.Now() と DB の now() は
	// 別の時計で、Docker のコンテナとホストでは1ms 程度ずれる。
	// 本番でも同じで、クライアントは自分の時計ではなく前回の serverTime を渡す
	// （openapi.yaml の updatedSince）。
	got, err := sync.Pull(ctx, s.UpdatedAt)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	found := false
	for _, x := range got.Sessions {
		if x.Id == s.Id {
			found = true
		}
	}
	if !found {
		t.Error("作成したセッションが差分に含まれていない")
	}
	if len(got.Sets) == 0 {
		t.Error("セットが差分に含まれていない")
	}
	// 次回の updatedSince に使うので、取得対象より後でなければならない
	if got.ServerTime.Before(s.UpdatedAt) {
		t.Errorf("serverTime %v が対象の updatedAt %v より前", got.ServerTime, s.UpdatedAt)
	}
}

// ADR-0014: 削除済みも返す。返さないとクライアント側で消えない
func TestPull_論理削除済みも返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 7, 2),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// 作成時点の updated_at を「前回同期」とする。time.Now() を使うと、
	// 削除が同じマイクロ秒に入ったときに境界で揺れる
	before := s.UpdatedAt
	if err := w.DeleteSession(ctx, s.Id); err != nil {
		t.Fatalf("DeleteSession: %v", err)
	}

	got, err := sync.Pull(ctx, before)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	for _, x := range got.Sessions {
		if x.Id == s.Id {
			if x.DeletedAt == nil {
				t.Error("deletedAt が入っていない。クライアントが削除と判別できない")
			}
			return
		}
	}
	t.Error("削除済みのセッションが差分に含まれていない")
}

func TestPull_期間より前の変更は返さない(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 7, 3),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	after := s.UpdatedAt.Add(time.Minute)
	got, err := sync.Pull(ctx, after)
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}
	for _, x := range got.Sessions {
		if x.Id == s.Id {
			t.Error("期間より前の変更が返っている")
		}
	}
}

// --- push ---

func TestPush_新規は適用される(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	id := testdb.RandomUUID()

	got, err := sync.Push(ctx, repository.PushInput{
		Sessions: []repository.SessionInput{{
			ID:   &id,
			Date: jstDate(2031, 8, 1),
			Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8, RIR: ptr(2)}},
		}},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if got.Applied != 1 {
		t.Errorf("applied = %d, want 1", got.Applied)
	}
	if len(got.Conflicts) != 0 {
		t.Errorf("conflicts = %+v", got.Conflicts)
	}
	if _, err := w.GetSession(ctx, id); err != nil {
		t.Errorf("作られていない: %v", err)
	}
}

// ADR-0014: サーバ側が新しければスキップして conflicts に入れる
func TestPush_サーバが新しければ競合として返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	s, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 8, 2),
		Note: ptr("サーバ側"),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// クライアントはサーバより古い時点の内容を持っている
	stale := s.UpdatedAt.Add(-time.Hour)
	got, err := sync.Push(ctx, repository.PushInput{
		Sessions: []repository.SessionInput{{
			ID: &s.Id, Date: jstDate(2031, 8, 2), Note: ptr("クライアント側"), UpdatedAt: &stale,
		}},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if got.Applied != 0 {
		t.Errorf("applied = %d, want 0", got.Applied)
	}
	if len(got.Conflicts) != 1 {
		t.Fatalf("conflicts = %d 件, want 1", len(got.Conflicts))
	}
	if got.Conflicts[0].Resource != "session" || got.Conflicts[0].ID != s.Id {
		t.Errorf("conflict = %+v", got.Conflicts[0])
	}

	// サーバ側の内容が残っている
	after, err := w.GetSession(ctx, s.Id)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if after.Note == nil || *after.Note != "サーバ側" {
		t.Errorf("note = %v, want サーバ側", after.Note)
	}
}

// 1件が競合しても他は適用される。全部止めると同期が進まなくなる
func TestPush_競合があっても他は適用される(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	tx := testdb.Begin(t)
	sync := repository.NewSync(tx)
	w := repository.NewWorkout(tx)
	ex := repository.NewExercise(tx)

	bench := firstExercise(t, ex, openapi.Chest)
	existing, err := w.CreateSession(ctx, repository.SessionInput{
		Date: jstDate(2031, 8, 3),
		Sets: []repository.SetInput{{ExerciseID: bench.Id, SetNo: 1, WeightKg: 80, Reps: 8}},
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	stale := existing.UpdatedAt.Add(-time.Hour)
	fresh := testdb.RandomUUID()

	got, err := sync.Push(ctx, repository.PushInput{
		Sessions: []repository.SessionInput{
			{ID: &existing.Id, Date: jstDate(2031, 8, 3), Note: ptr("古い"), UpdatedAt: &stale},
			{ID: &fresh, Date: jstDate(2031, 8, 4)},
		},
	})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if got.Applied != 1 {
		t.Errorf("applied = %d, want 1", got.Applied)
	}
	if len(got.Conflicts) != 1 {
		t.Errorf("conflicts = %d 件, want 1", len(got.Conflicts))
	}
	if _, err := w.GetSession(ctx, fresh); err != nil {
		t.Errorf("競合していない方が作られていない: %v", err)
	}
}

// serverTime は次回の updatedSince としてクエリ文字列に載る。
// JST の "+09:00" はエンコードを忘れると "+" が空白に解釈されて壊れる
func TestPull_serverTimeはUTCで返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	sync := repository.NewSync(testdb.Begin(t))

	got, err := sync.Pull(ctx, time.Now().Add(-time.Hour))
	if err != nil {
		t.Fatalf("Pull: %v", err)
	}

	if _, offset := got.ServerTime.Zone(); offset != 0 {
		t.Errorf("serverTime のオフセット = %d秒, want 0（UTC）: %v", offset, got.ServerTime.Format(time.RFC3339))
	}
}

func TestPush_serverTimeはUTCで返す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	sync := repository.NewSync(testdb.Begin(t))

	got, err := sync.Push(ctx, repository.PushInput{})
	if err != nil {
		t.Fatalf("Push: %v", err)
	}

	if _, offset := got.ServerTime.Zone(); offset != 0 {
		t.Errorf("serverTime のオフセット = %d秒, want 0（UTC）", offset)
	}
}
