package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Sync はオフライン同期（ADR-0014）。
type Sync struct {
	db DBTX
}

// NewSync は Sync を作る。
func NewSync(db DBTX) *Sync {
	return &Sync{db: db}
}

// PullResult は差分取得の結果。
type PullResult struct {
	ServerTime time.Time
	Exercises  []openapi.Exercise
	Sessions   []openapi.WorkoutSession
	Sets       []openapi.WorkoutSet
	Templates  []openapi.Template
}

// Pull は updatedSince 以降に変更されたものを返す。
//
// since には **サーバが前回返した ServerTime を渡す**（openapi.yaml の updatedSince）。
// クライアント自身の時計を使ってはいけない。端末とサーバの時計は数ミリ秒〜数秒ずれ、
// ずれの向き次第で変更を取りこぼす。
//
// 境界は **>= にする**（> ではない）。Postgres の timestamptz はマイクロ秒精度で、
// 前回の serverTime と同じマイクロ秒に書かれた行は > だと取りこぼす。
// **同期では変更を落とす方が再送より悪い。** クライアント側の適用は
// 冪等（id で upsert）なので、境界の行が毎回1件返っても害が無い。
//
// **削除済み（deleted_at が入ったもの）も含む。** 含めないと、クライアント側で
// 削除が反映されず残り続ける（ADR-0014）。
//
// セッションはセットを入れ子にせず、sets として並列に返す。クライアントは
// ローカル DB のテーブルに素直に流し込める。
func (r *Sync) Pull(ctx context.Context, since time.Time) (PullResult, error) {
	// serverTime はクエリの前に取る。取得中に入った変更を次回に拾えるようにするため。
	// 後に取ると、その間の変更を「取得済み」として飛ばしてしまう。
	//
	// **UTC で返す。** これは表示用の時刻ではなく次回の updatedSince に渡す
	// カーソルで、クエリ文字列に載る。JST の "+09:00" はエンコードを忘れると
	// "+" が空白に解釈されて壊れる（ADR-0013 は表示の話であって、ここは別）
	out := PullResult{ServerTime: timeutil.Now().UTC()}

	var err error
	if out.Exercises, err = r.pullExercises(ctx, since); err != nil {
		return PullResult{}, err
	}
	if out.Sessions, err = r.pullSessions(ctx, since); err != nil {
		return PullResult{}, err
	}
	if out.Sets, err = r.pullSets(ctx, since); err != nil {
		return PullResult{}, err
	}
	if out.Templates, err = r.pullTemplates(ctx, since); err != nil {
		return PullResult{}, err
	}

	return out, nil
}

func (r *Sync) pullExercises(ctx context.Context, since time.Time) ([]openapi.Exercise, error) {
	const q = `select ` + exerciseColumns + `
		from exercises where updated_at >= $1 order by updated_at`

	return collect(ctx, r.db, q, since, scanExercise)
}

func (r *Sync) pullSessions(ctx context.Context, since time.Time) ([]openapi.WorkoutSession, error) {
	const q = `select ` + sessionColumns + `
		from workout_sessions where updated_at >= $1 order by updated_at`

	return collect(ctx, r.db, q, since, scanSession)
}

func (r *Sync) pullSets(ctx context.Context, since time.Time) ([]openapi.WorkoutSet, error) {
	const q = `select ` + setColumns + `
		from workout_sets where updated_at >= $1 order by updated_at`

	return collect(ctx, r.db, q, since, scanSet)
}

func (r *Sync) pullTemplates(ctx context.Context, since time.Time) ([]openapi.Template, error) {
	const q = `select ` + templateColumns + `
		from templates where updated_at >= $1 order by updated_at`

	return collect(ctx, r.db, q, since, scanTemplate)
}

// collect は同じ形の「差分を引いて詰める」を1箇所にまとめる。
func collect[T any](ctx context.Context, db DBTX, q string, since time.Time,
	scan func(row) (T, error),
) ([]T, error) {
	rows, err := db.Query(ctx, q, since)
	if err != nil {
		return nil, fmt.Errorf("差分を引けない: %w", err)
	}
	defer rows.Close()

	out := make([]T, 0, 64)
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return nil, fmt.Errorf("差分を読めない: %w", err)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("差分を読めない: %w", err)
	}

	return out, nil
}

// --- push ---

// PushInput はクライアントが溜めた変更。
type PushInput struct {
	Sessions []SessionInput
	Sets     []SetInput
}

// Conflict は適用されなかったもの。
type Conflict struct {
	// Resource は "session" か "set"
	Resource        string
	ID              uuid.UUID
	ServerUpdatedAt time.Time
}

// PushResult は一括送信の結果。
type PushResult struct {
	Applied    int
	Conflicts  []Conflict
	ServerTime time.Time
}

// Push はクライアントの変更を適用する。
//
// サーバ側の updated_at が新しいものはスキップし、conflicts に入れて返す
// （Last Write Wins / ADR-0014）。
//
// **1件の競合で全部を止めない。** 止めると、1つ古い変更を抱えたクライアントが
// 永久に同期できなくなる。
func (r *Sync) Push(ctx context.Context, in PushInput) (PushResult, error) {
	out := PushResult{Conflicts: []Conflict{}}

	w := NewWorkout(r.db)

	for _, s := range in.Sessions {
		applied, conflict, err := r.pushSession(ctx, w, s)
		if err != nil {
			return PushResult{}, err
		}
		if conflict != nil {
			out.Conflicts = append(out.Conflicts, *conflict)
			continue
		}
		if applied {
			out.Applied++
		}
	}

	for _, s := range in.Sets {
		applied, conflict, err := r.pushSet(ctx, w, s)
		if err != nil {
			return PushResult{}, err
		}
		if conflict != nil {
			out.Conflicts = append(out.Conflicts, *conflict)
			continue
		}
		if applied {
			out.Applied++
		}
	}

	// serverTime は適用の後に取る。ここまでの変更をクライアントが
	// 次回の pull で拾い直さなくて済むようにするため。UTC で返す理由は Pull と同じ
	out.ServerTime = timeutil.Now().UTC()

	return out, nil
}

func (r *Sync) pushSession(ctx context.Context, w *Workout, in SessionInput) (bool, *Conflict, error) {
	if in.ID == nil {
		// id 無しは新規。冪等にできないが、クライアントが UUID を作るのが前提
		if _, err := w.CreateSession(ctx, in); err != nil {
			return false, nil, err
		}
		return true, nil, nil
	}

	existing, err := w.GetSession(ctx, *in.ID)
	if IsNotFound(err) {
		if _, err := w.CreateSession(ctx, in); err != nil {
			return false, nil, err
		}
		return true, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	// クライアントが updatedAt を持っていて、サーバの方が新しければ競合
	if in.UpdatedAt != nil && existing.UpdatedAt.After(*in.UpdatedAt) {
		return false, &Conflict{
			Resource: "session", ID: *in.ID, ServerUpdatedAt: existing.UpdatedAt,
		}, nil
	}

	_, err = w.UpdateSession(ctx, *in.ID, SessionUpdate{
		Date: &in.Date, TemplateID: in.TemplateID, Note: in.Note,
	})
	if err != nil {
		return false, nil, err
	}

	return true, nil, nil
}

func (r *Sync) pushSet(ctx context.Context, w *Workout, in SetInput) (bool, *Conflict, error) {
	if in.ID == nil || in.SessionID == nil {
		return false, nil, errors.New("セットの同期には id と sessionId が要る")
	}

	existing, err := w.getSet(ctx, *in.ID)
	if IsNotFound(err) {
		if _, err := w.CreateSet(ctx, *in.SessionID, in); err != nil {
			return false, nil, err
		}
		return true, nil, nil
	}
	if err != nil {
		return false, nil, err
	}

	if in.UpdatedAt != nil && existing.UpdatedAt.After(*in.UpdatedAt) {
		return false, &Conflict{
			Resource: "set", ID: *in.ID, ServerUpdatedAt: existing.UpdatedAt,
		}, nil
	}

	if _, err := w.UpdateSet(ctx, *in.ID, in); err != nil {
		return false, nil, err
	}

	return true, nil, nil
}
