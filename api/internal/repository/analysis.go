package repository

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// Analysis は分析用の読み取り。MCP から使う（ADR-0010）。
type Analysis struct {
	db DBTX
}

// NewAnalysis は Analysis を作る。
func NewAnalysis(db DBTX) *Analysis {
	return &Analysis{db: db}
}

// MuscleVolume は部位ごとの週間ボリューム。
type MuscleVolume struct {
	MuscleGroup openapi.MuscleGroup
	Sets        int
	TonnageKg   float64
}

// WeeklyVolume は期間内の部位別セット数とトン数を返す。
//
// 「有効セット数」は単純なセット数。ウォームアップを除く仕組みは持っていない
// （RIR で区別できるが、記録の運用が固まってから入れる）。
func (r *Analysis) WeeklyVolume(ctx context.Context, from, to openapi_types.Date) ([]MuscleVolume, error) {
	const q = `
		select e.muscle_group,
		       count(*)::int as sets,
		       coalesce(sum(s.weight_kg * s.reps), 0)::float8 as tonnage
		from workout_sets s
		join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		join exercises e on e.id = s.exercise_id
		where s.deleted_at is null
		  and ws.date between $1::date and $2::date
		group by e.muscle_group
		order by sets desc`

	rows, err := r.db.Query(ctx, q, from.Time, to.Time)
	if err != nil {
		return nil, fmt.Errorf("部位別ボリュームを引けない: %w", err)
	}
	defer rows.Close()

	out := make([]MuscleVolume, 0, 13)
	for rows.Next() {
		var (
			v  MuscleVolume
			mg string
		)
		if err := rows.Scan(&mg, &v.Sets, &v.TonnageKg); err != nil {
			return nil, fmt.Errorf("部位別ボリュームを読めない: %w", err)
		}
		v.MuscleGroup = openapi.MuscleGroup(mg)
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("部位別ボリュームを読めない: %w", err)
	}

	return out, nil
}

// E1RMPoint は1日分の推定1RM。
type E1RMPoint struct {
	Date     openapi_types.Date
	BestE1RM float64
	Sets     int
}

// ExerciseHistory は種目の日別の最大 e1RM を古い順に返す。
//
// 傾きの回帰に使うので昇順。計算できないセット（RIR 未記録・限界12レップ超）は
// 除外し、その日に計算できるセットが1つも無ければ点を作らない。
func (r *Analysis) ExerciseHistory(ctx context.Context, exerciseID uuid.UUID, from, to openapi_types.Date) ([]E1RMPoint, error) {
	// e1RM の計算は Go 側でやる。SQL に式を書くと analytics と二重管理になり、
	// 「Python と一致すること」の検証対象が増える
	const q = `
		select ws.date, s.weight_kg, s.reps, s.rir
		from workout_sets s
		join workout_sessions ws on ws.id = s.session_id and ws.deleted_at is null
		where s.exercise_id = $1
		  and s.deleted_at is null
		  and ws.date between $2::date and $3::date
		order by ws.date`

	rows, err := r.db.Query(ctx, q, exerciseID, from.Time, to.Time)
	if err != nil {
		return nil, fmt.Errorf("種目の履歴を引けない: %w", err)
	}
	defer rows.Close()

	byDate := map[time.Time]*E1RMPoint{}
	order := []time.Time{}

	for rows.Next() {
		var (
			date   time.Time
			weight float32
			reps   int
			rir    *int
		)
		if err := rows.Scan(&date, &weight, &reps, &rir); err != nil {
			return nil, fmt.Errorf("種目の履歴を読めない: %w", err)
		}
		if !analytics.UsableForE1RM(reps, rir) {
			continue
		}

		v := analytics.E1RM(float64(weight), float64(reps), float64(*rir))
		p, ok := byDate[date]
		if !ok {
			p = &E1RMPoint{Date: openapi_types.Date{Time: date}}
			byDate[date] = p
			order = append(order, date)
		}
		p.Sets++
		if v > p.BestE1RM {
			p.BestE1RM = v
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("種目の履歴を読めない: %w", err)
	}

	out := make([]E1RMPoint, 0, len(order))
	for _, d := range order {
		out = append(out, *byDate[d])
	}

	return out, nil
}

// --- 読み取り専用クエリ ---

// QueryResult は任意クエリの結果。
type QueryResult struct {
	Columns []string
	Rows    [][]any

	// Truncated は limit で打ち切ったか
	Truncated bool
}

// ErrWriteQuery は書き込みを含むクエリであることを表す。
var ErrWriteQuery = errors.New("読み取り専用のクエリだけ実行できる")

// 書き込み・DDL・権限変更のキーワード。
// 単語境界で見る（"created_at" のような列名に反応しないため）。
var writeKeywords = regexp.MustCompile(
	`(?i)\b(insert|update|delete|drop|truncate|alter|create|grant|revoke|comment|copy|vacuum|reindex|call|do|merge|refresh|set|reset|listen|notify|lock)\b`)

// Query は読み取り専用のクエリを実行する（MCP 用）。
//
// **事前に定義できない質問に答えるための口**（ADR-0010）。
// 「睡眠6時間未満だった翌日の e1RM」のような質問は固定のツールでは扱えない。
//
// 安全側に倒すため、次の3段で守る。
//  1. select / with で始まることを要求する
//  2. 書き込み系のキーワードを含むものを弾く（CTE に隠した delete も）
//  3. 複数文（セミコロン区切り）を弾く
//
// **これでも完全ではない。** 本来は接続自体を読み取り専用ロールにするのが正しく、
// MCP を外部に公開するなら必須になる（issue に切り出す）。
func (r *Analysis) Query(ctx context.Context, sql string, limit int) (QueryResult, error) {
	if err := checkReadOnly(sql); err != nil {
		return QueryResult{}, err
	}
	if limit <= 0 {
		limit = 100
	}

	rows, err := r.db.Query(ctx, sql)
	if err != nil {
		// MCP 経由で Claude が自分で直せるよう、DB のエラーをそのまま返す
		return QueryResult{}, fmt.Errorf("クエリが失敗した: %w", err)
	}
	defer rows.Close()

	out := QueryResult{Rows: [][]any{}}
	for _, fd := range rows.FieldDescriptions() {
		out.Columns = append(out.Columns, fd.Name)
	}

	for rows.Next() {
		if len(out.Rows) >= limit {
			out.Truncated = true
			break
		}
		vals, err := rows.Values()
		if err != nil {
			return QueryResult{}, fmt.Errorf("結果を読めない: %w", err)
		}
		out.Rows = append(out.Rows, vals)
	}
	if err := rows.Err(); err != nil && !out.Truncated {
		return QueryResult{}, fmt.Errorf("結果を読めない: %w", err)
	}

	return out, nil
}

func checkReadOnly(sql string) error {
	trimmed := strings.TrimSpace(sql)
	if trimmed == "" {
		return fmt.Errorf("%w: 空のクエリ", ErrWriteQuery)
	}

	// 複数文を弾く。末尾のセミコロン1つだけは許す
	body := strings.TrimSuffix(trimmed, ";")
	if strings.Contains(body, ";") {
		return fmt.Errorf("%w: 複数の文は実行できない", ErrWriteQuery)
	}

	lower := strings.ToLower(body)
	if !strings.HasPrefix(lower, "select") && !strings.HasPrefix(lower, "with") {
		return fmt.Errorf("%w: select か with で始める必要がある", ErrWriteQuery)
	}

	// with x as (delete ... returning ...) select * from x を弾く
	if m := writeKeywords.FindString(body); m != "" {
		return fmt.Errorf("%w: %q を含んでいる", ErrWriteQuery, m)
	}

	return nil
}
