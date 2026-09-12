package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// Workout はトレーニング記録へのアクセス。
type Workout struct {
	db DBTX
}

// NewWorkout は Workout を作る。
func NewWorkout(db DBTX) *Workout {
	return &Workout{db: db}
}

// SetInput はセット1本の入力。
type SetInput struct {
	ID         *uuid.UUID
	ExerciseID uuid.UUID
	SetNo      int
	WeightKg   float32

	Reps int

	// RIR は nil を許す。過去データの取り込み（要件 I-01）では欠損がありうる。
	// ただし無いと推定1RMが計算できない
	RIR *int
}

// SessionInput はセッション作成の入力。
type SessionInput struct {
	// ID はクライアント生成の UUID。nil ならサーバが採番する
	ID         *uuid.UUID
	Date       openapi_types.Date
	TemplateID *uuid.UUID
	Note       *string

	// Sets は一括登録するセット。空でもよい（後から足せる）
	Sets []SetInput
}

// SessionUpdate はセッション更新の入力。nil のフィールドは変更しない。
type SessionUpdate struct {
	Date       *openapi_types.Date
	TemplateID *uuid.UUID
	Note       *string

	// ExpectedUpdatedAt はクライアントが保持する更新時刻。
	// サーバ側がこれより新しければ ErrConflict（ADR-0014）。
	// nil なら競合を検出せずに上書きする
	ExpectedUpdatedAt *time.Time
}

// SessionFilter は一覧の絞り込み条件。
type SessionFilter struct {
	From  *openapi_types.Date
	To    *openapi_types.Date
	Limit int
}

const sessionColumns = `id, date, template_id, note, created_at, updated_at, deleted_at`

const setColumns = `id, session_id, exercise_id, set_no, weight_kg, reps, rir,
	created_at, updated_at, deleted_at`

// join するクエリでは列名が衝突するので別名を付けたものを使う
const setColumnsPrefixed = `s.id, s.session_id, s.exercise_id, s.set_no, s.weight_kg, s.reps, s.rir,
	s.created_at, s.updated_at, s.deleted_at`

// defaultSessionLimit は limit 未指定時の件数。
const defaultSessionLimit = 50

// CreateSession はセッションを作る。Sets が指定されていれば同時に登録する。
func (r *Workout) CreateSession(ctx context.Context, in SessionInput) (openapi.WorkoutSession, error) {
	s, _, err := r.CreateSessionIdempotent(ctx, in)
	return s, err
}

// CreateSessionIdempotent はセッションを作る。
// ID 指定で既に存在した場合は既存を返し、existed に true を返す
// （openapi.yaml が 201 と 200 を出し分けるため）。
func (r *Workout) CreateSessionIdempotent(ctx context.Context, in SessionInput) (openapi.WorkoutSession, bool, error) {
	const q = `
		insert into workout_sessions (id, date, template_id, note)
		values (coalesce($1, gen_random_uuid()), $2, $3, $4)
		on conflict (id) do nothing
		returning ` + sessionColumns

	s, err := scanSession(r.db.QueryRow(ctx, q, in.ID, in.Date.Time, in.TemplateID, in.Note))
	switch {
	case errors.Is(err, pgx.ErrNoRows) && in.ID != nil:
		// 同じ id での再送。セットも作り直さず、既存をそのまま返す。
		// do update にすると再送のたびに updated_at が進み、競合解決が誤作動する
		existing, err := r.GetSession(ctx, *in.ID)
		return existing, true, err
	case err != nil:
		return openapi.WorkoutSession{}, false, fmt.Errorf("セッションを作れない: %w", err)
	}

	s.Sets = make([]openapi.WorkoutSet, 0, len(in.Sets))
	for _, si := range in.Sets {
		set, err := r.insertSet(ctx, s.Id, si)
		if err != nil {
			return openapi.WorkoutSession{}, false, err
		}
		s.Sets = append(s.Sets, set)
	}

	return s, false, nil
}

// GetSession はセッションをセット込みで返す。論理削除済みは ErrNotFound。
func (r *Workout) GetSession(ctx context.Context, id uuid.UUID) (openapi.WorkoutSession, error) {
	const q = `select ` + sessionColumns + `
		from workout_sessions where id = $1 and deleted_at is null`

	s, err := scanSession(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.WorkoutSession{}, fmt.Errorf("セッション %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.WorkoutSession{}, fmt.Errorf("セッションを取得できない: %w", err)
	}

	s.Sets, err = r.listSets(ctx, id)
	if err != nil {
		return openapi.WorkoutSession{}, err
	}

	return s, nil
}

// ListSessions は条件に合うセッションを新しい順に返す。セットを含む。
func (r *Workout) ListSessions(ctx context.Context, f SessionFilter) ([]openapi.WorkoutSession, error) {
	limit := f.Limit
	if limit <= 0 {
		limit = defaultSessionLimit
	}

	const q = `
		select ` + sessionColumns + `
		from workout_sessions
		where deleted_at is null
		  and ($1::date is null or date >= $1::date)
		  and ($2::date is null or date <= $2::date)
		order by date desc, created_at desc
		limit $3`

	rows, err := r.db.Query(ctx, q, dateOrNil(f.From), dateOrNil(f.To), limit)
	if err != nil {
		return nil, fmt.Errorf("セッションの一覧を引けない: %w", err)
	}
	defer rows.Close()

	sessions := make([]openapi.WorkoutSession, 0, limit)
	for rows.Next() {
		s, err := scanSession(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("セッションの一覧を読めない: %w", err)
	}

	// N+1 になるが、limit が既定 50 で、1セッションあたりのセットは
	// 1日4種目 × 4セット程度。まとめて引くより読みやすさを採る
	for i := range sessions {
		sessions[i].Sets, err = r.listSets(ctx, sessions[i].Id)
		if err != nil {
			return nil, err
		}
	}

	return sessions, nil
}

// UpdateSession はセッションを更新する。
func (r *Workout) UpdateSession(ctx context.Context, id uuid.UUID, up SessionUpdate) (openapi.WorkoutSession, error) {
	// $5 が null なら競合を見ない。指定されていて、サーバ側の updated_at が
	// それより新しければ 0 行になる（ADR-0014 の Last Write Wins）
	const q = `
		update workout_sessions
		set date = coalesce($2, date),
		    template_id = coalesce($3, template_id),
		    note = coalesce($4, note),
		    updated_at = $6
		where id = $1
		  and deleted_at is null
		  and ($5::timestamptz is null or updated_at <= $5::timestamptz)
		returning ` + sessionColumns

	var date *time.Time
	if up.Date != nil {
		date = &up.Date.Time
	}

	s, err := scanSession(r.db.QueryRow(ctx, q, id, date, up.TemplateID, up.Note, up.ExpectedUpdatedAt, timeutil.Now()))
	if errors.Is(err, pgx.ErrNoRows) {
		// 0 行の理由が「存在しない」か「競合」かを区別する
		if up.ExpectedUpdatedAt != nil {
			if _, gerr := r.GetSession(ctx, id); gerr == nil {
				return openapi.WorkoutSession{}, fmt.Errorf("セッション %s はサーバ側が新しい: %w", id, ErrConflict)
			}
		}
		return openapi.WorkoutSession{}, fmt.Errorf("セッション %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.WorkoutSession{}, fmt.Errorf("セッションを更新できない: %w", err)
	}

	s.Sets, err = r.listSets(ctx, id)
	if err != nil {
		return openapi.WorkoutSession{}, err
	}

	return s, nil
}

// DeleteSession はセッションと配下のセットを論理削除する（ADR-0014）。
func (r *Workout) DeleteSession(ctx context.Context, id uuid.UUID) error {
	now := timeutil.Now()

	const q = `
		update workout_sessions set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, now)
	if err != nil {
		return fmt.Errorf("セッションを削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("セッション %s: %w", id, ErrNotFound)
	}

	// セッションだけ消してセットを残すと、同期でセットだけが降ってくる
	const qs = `
		update workout_sets set deleted_at = $2, updated_at = $2
		where session_id = $1 and deleted_at is null`

	if _, err := r.db.Exec(ctx, qs, id, now); err != nil {
		return fmt.Errorf("配下のセットを削除できない: %w", err)
	}

	return nil
}

// CreateSet はセッションにセットを追加する。
func (r *Workout) CreateSet(ctx context.Context, sessionID uuid.UUID, in SetInput) (openapi.WorkoutSet, error) {
	// 存在しないセッションへの追加を 404 にする。外部キー違反のままだと 500 になる
	if _, err := r.GetSession(ctx, sessionID); err != nil {
		return openapi.WorkoutSet{}, err
	}

	return r.insertSet(ctx, sessionID, in)
}

// UpdateSet はセットを更新する。
func (r *Workout) UpdateSet(ctx context.Context, id uuid.UUID, in SetInput) (openapi.WorkoutSet, error) {
	const q = `
		update workout_sets
		set exercise_id = $2, set_no = $3, weight_kg = $4, reps = $5, rir = $6, updated_at = $7
		where id = $1 and deleted_at is null
		returning ` + setColumns

	s, err := scanSet(r.db.QueryRow(ctx, q, id, in.ExerciseID, in.SetNo, in.WeightKg, in.Reps, in.RIR, timeutil.Now()))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return openapi.WorkoutSet{}, fmt.Errorf("セット %s: %w", id, ErrNotFound)
	case isUniqueViolation(err):
		return openapi.WorkoutSet{}, fmt.Errorf("セット番号 %d: %w", in.SetNo, ErrConflict)
	case err != nil:
		return openapi.WorkoutSet{}, fmt.Errorf("セットを更新できない: %w", err)
	}

	return s, nil
}

// DeleteSet はセットを論理削除する。
func (r *Workout) DeleteSet(ctx context.Context, id uuid.UUID) error {
	const q = `
		update workout_sets set deleted_at = $2, updated_at = $2
		where id = $1 and deleted_at is null`

	tag, err := r.db.Exec(ctx, q, id, timeutil.Now())
	if err != nil {
		return fmt.Errorf("セットを削除できない: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("セット %s: %w", id, ErrNotFound)
	}

	return nil
}

func (r *Workout) insertSet(ctx context.Context, sessionID uuid.UUID, in SetInput) (openapi.WorkoutSet, error) {
	const q = `
		insert into workout_sets (id, session_id, exercise_id, set_no, weight_kg, reps, rir)
		values (coalesce($1, gen_random_uuid()), $2, $3, $4, $5, $6, $7)
		on conflict (id) do nothing
		returning ` + setColumns

	s, err := scanSet(r.db.QueryRow(ctx, q, in.ID, sessionID, in.ExerciseID, in.SetNo, in.WeightKg, in.Reps, in.RIR))
	switch {
	case errors.Is(err, pgx.ErrNoRows) && in.ID != nil:
		return r.getSet(ctx, *in.ID)
	case isUniqueViolation(err):
		return openapi.WorkoutSet{}, fmt.Errorf("セット番号 %d は既にある: %w", in.SetNo, ErrConflict)
	case err != nil:
		return openapi.WorkoutSet{}, fmt.Errorf("セットを作れない: %w", err)
	}

	return s, nil
}

func (r *Workout) getSet(ctx context.Context, id uuid.UUID) (openapi.WorkoutSet, error) {
	const q = `select ` + setColumns + ` from workout_sets where id = $1`

	s, err := scanSet(r.db.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return openapi.WorkoutSet{}, fmt.Errorf("セット %s: %w", id, ErrNotFound)
	}
	if err != nil {
		return openapi.WorkoutSet{}, fmt.Errorf("セットを取得できない: %w", err)
	}

	return s, nil
}

func (r *Workout) listSets(ctx context.Context, sessionID uuid.UUID) ([]openapi.WorkoutSet, error) {
	const q = `
		select ` + setColumns + `
		from workout_sets
		where session_id = $1 and deleted_at is null
		order by set_no`

	rows, err := r.db.Query(ctx, q, sessionID)
	if err != nil {
		return nil, fmt.Errorf("セットの一覧を引けない: %w", err)
	}
	defer rows.Close()

	sets := make([]openapi.WorkoutSet, 0, 16)
	for rows.Next() {
		s, err := scanSet(rows)
		if err != nil {
			return nil, err
		}
		sets = append(sets, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("セットの一覧を読めない: %w", err)
	}

	return sets, nil
}

func scanSession(r row) (openapi.WorkoutSession, error) {
	var (
		s    openapi.WorkoutSession
		date time.Time
	)
	err := r.Scan(&s.Id, &date, &s.TemplateId, &s.Note, &s.CreatedAt, &s.UpdatedAt, &s.DeletedAt)
	if err != nil {
		return openapi.WorkoutSession{}, err
	}

	// date 列は JST における日付そのもの（ADR-0013）。タイムゾーン変換しない
	s.Date = openapi_types.Date{Time: date}
	s.CreatedAt = s.CreatedAt.In(timeutil.JST)
	s.UpdatedAt = s.UpdatedAt.In(timeutil.JST)
	if s.DeletedAt != nil {
		jst := s.DeletedAt.In(timeutil.JST)
		s.DeletedAt = &jst
	}
	s.Sets = []openapi.WorkoutSet{}

	return s, nil
}

func scanSet(r row) (openapi.WorkoutSet, error) {
	var s openapi.WorkoutSet
	err := r.Scan(&s.Id, &s.SessionId, &s.ExerciseId, &s.SetNo, &s.WeightKg, &s.Reps, &s.Rir,
		&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt)
	if err != nil {
		return openapi.WorkoutSet{}, err
	}

	s.CreatedAt = s.CreatedAt.In(timeutil.JST)
	s.UpdatedAt = s.UpdatedAt.In(timeutil.JST)
	if s.DeletedAt != nil {
		jst := s.DeletedAt.In(timeutil.JST)
		s.DeletedAt = &jst
	}

	return s, nil
}

func dateOrNil(d *openapi_types.Date) *time.Time {
	if d == nil {
		return nil
	}

	return &d.Time
}

// LastPerformanceResult は前回の実施内容。
type LastPerformanceResult struct {
	// Date は前回実施日。一度も実施していなければ nil
	Date *openapi_types.Date
	Sets []openapi.WorkoutSet
}

// LastPerformance は種目の前回実施内容を返す（要件 T-02）。
//
// 入力速度を決める最重要機能なので、1クエリで取る。
// 論理削除されたセッション・セットは対象外。
func (r *Workout) LastPerformance(ctx context.Context, exerciseID uuid.UUID) (LastPerformanceResult, error) {
	// 直近の実施日を先に決めてから、その日のセットを引く。
	// 「最新のセット N 件」にすると、日をまたいだセットが混ざる
	const q = `
		with last as (
			select ws.id, ws.date
			from workout_sessions ws
			join workout_sets s on s.session_id = ws.id and s.deleted_at is null
			where s.exercise_id = $1 and ws.deleted_at is null
			order by ws.date desc, ws.created_at desc
			limit 1
		)
		select last.date, ` + setColumnsPrefixed + `
		from last
		join workout_sets s on s.session_id = last.id
		where s.exercise_id = $1 and s.deleted_at is null
		order by s.set_no`

	rows, err := r.db.Query(ctx, q, exerciseID)
	if err != nil {
		return LastPerformanceResult{}, fmt.Errorf("前回値を引けない: %w", err)
	}
	defer rows.Close()

	out := LastPerformanceResult{Sets: []openapi.WorkoutSet{}}
	for rows.Next() {
		var (
			date time.Time
			s    openapi.WorkoutSet
		)
		err := rows.Scan(&date, &s.Id, &s.SessionId, &s.ExerciseId, &s.SetNo, &s.WeightKg, &s.Reps, &s.Rir,
			&s.CreatedAt, &s.UpdatedAt, &s.DeletedAt)
		if err != nil {
			return LastPerformanceResult{}, fmt.Errorf("前回値を読めない: %w", err)
		}

		if out.Date == nil {
			out.Date = &openapi_types.Date{Time: date}
		}
		s.CreatedAt = s.CreatedAt.In(timeutil.JST)
		s.UpdatedAt = s.UpdatedAt.In(timeutil.JST)
		out.Sets = append(out.Sets, s)
	}
	if err := rows.Err(); err != nil {
		return LastPerformanceResult{}, fmt.Errorf("前回値を読めない: %w", err)
	}

	return out, nil
}
