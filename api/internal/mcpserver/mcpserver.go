// Package mcpserver は Claude Code から分析するための MCP サーバ。
//
// **分析UIを作り込まず、ここから問い合わせるのが主**（ADR-0010）。
// 「睡眠6時間未満だった翌日の e1RM は平均でどれくらい落ちるか」のような質問は
// データが溜まって仮説が立ってから生まれるので、固定の画面では構造的に扱えない。
//
// DB を直接読む。API を経由しないのは、MCP が手元でしか動かないため
// （認証を挟む意味が無く、CSV エクスポートの往復も要らない）。
package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// maxQueryRows は任意クエリで返す最大行数。
// これを超えると Claude のコンテキストを圧迫するだけで役に立たない。
const maxQueryRows = 200

// Deps は MCP サーバが使う読み取り口。
type Deps struct {
	Exercises *repository.Exercise
	Workouts  *repository.Workout
	Analysis  *repository.Analysis

	// Plan は3年計画の設定。**DB を正とする**（要件 P-05）。
	// nil でも他のツールは動く。weekly_actions だけが要求する
	Plan *repository.Plan
}

// New は MCP サーバを組み立てる。
func New(d Deps) *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{
		Name:    "physique-analytics",
		Version: "0.1.0",
	}, nil)

	registerExerciseTools(s, d)
	registerWorkoutTools(s, d)
	registerAnalysisTools(s, d)
	registerWeeklyTools(s, d)

	return s
}

// --- 種目 ---

type listExercisesIn struct {
	MuscleGroup string `json:"muscleGroup,omitempty" jsonschema:"部位で絞る。胸/広背筋/僧帽筋/肩前部/肩中部/肩後部/上腕二頭/上腕三頭/大腿四頭/ハム/臀部/ふくらはぎ/腹"`
}

type exerciseOut struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	MuscleGroup string `json:"muscleGroup"`
	IsCompound  bool   `json:"isCompound"`
}

type listExercisesOut struct {
	Items []exerciseOut `json:"items"`
}

func registerExerciseTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_exercises",
		Description: "種目マスタの一覧。種目 ID は他のツールの引数に使う。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listExercisesIn) (*mcp.CallToolResult, listExercisesOut, error) {
		f := repository.ExerciseFilter{}
		if in.MuscleGroup != "" {
			mg := openapi.MuscleGroup(in.MuscleGroup)
			if !mg.Valid() {
				return nil, listExercisesOut{}, fmt.Errorf("未知の部位: %q", in.MuscleGroup)
			}
			f.MuscleGroup = &mg
		}

		items, err := d.Exercises.List(ctx, f)
		if err != nil {
			return nil, listExercisesOut{}, err
		}

		out := listExercisesOut{Items: make([]exerciseOut, 0, len(items))}
		for _, e := range items {
			out.Items = append(out.Items, exerciseOut{
				ID:          e.Id.String(),
				Name:        e.Name,
				MuscleGroup: string(e.MuscleGroup),
				IsCompound:  e.IsCompound != nil && *e.IsCompound,
			})
		}

		return nil, out, nil
	})
}

// --- トレーニング記録 ---

type listSessionsIn struct {
	From  string `json:"from,omitempty" jsonschema:"開始日 YYYY-MM-DD（JST）。省略時は30日前"`
	To    string `json:"to,omitempty" jsonschema:"終了日 YYYY-MM-DD（JST）。省略時は今日"`
	Limit int    `json:"limit,omitempty" jsonschema:"最大件数。既定 50"`
}

type setOut struct {
	SetNo      int     `json:"setNo"`
	ExerciseID string  `json:"exerciseId"`
	WeightKg   float32 `json:"weightKg"`
	Reps       int     `json:"reps"`
	RIR        *int    `json:"rir,omitempty"`
}

type sessionOut struct {
	ID   string   `json:"id"`
	Date string   `json:"date"`
	Note *string  `json:"note,omitempty"`
	Sets []setOut `json:"sets"`
}

type listSessionsOut struct {
	Items []sessionOut `json:"items"`
}

func registerWorkoutTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name:        "list_workout_sessions",
		Description: "トレーニング記録を期間で取る。新しい順。セットを含む。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in listSessionsIn) (*mcp.CallToolResult, listSessionsOut, error) {
		from, to, err := dateRange(in.From, in.To, 30)
		if err != nil {
			return nil, listSessionsOut{}, err
		}

		items, err := d.Workouts.ListSessions(ctx, repository.SessionFilter{From: &from, To: &to, Limit: in.Limit})
		if err != nil {
			return nil, listSessionsOut{}, err
		}

		out := listSessionsOut{Items: make([]sessionOut, 0, len(items))}
		for _, s := range items {
			so := sessionOut{
				ID:   s.Id.String(),
				Date: s.Date.Format("2006-01-02"),
				Note: s.Note,
				Sets: make([]setOut, 0, len(s.Sets)),
			}
			for _, set := range s.Sets {
				so.Sets = append(so.Sets, setOut{
					SetNo: set.SetNo, ExerciseID: set.ExerciseId.String(),
					WeightKg: set.WeightKg, Reps: set.Reps, RIR: set.Rir,
				})
			}
			out.Items = append(out.Items, so)
		}

		return nil, out, nil
	})
}

// --- 分析 ---

type weeklyVolumeIn struct {
	From string `json:"from,omitempty" jsonschema:"開始日 YYYY-MM-DD（JST）。省略時は7日前"`
	To   string `json:"to,omitempty" jsonschema:"終了日 YYYY-MM-DD（JST）。省略時は今日"`
}

type volumeOut struct {
	MuscleGroup string  `json:"muscleGroup"`
	Sets        int     `json:"sets"`
	TonnageKg   float64 `json:"tonnageKg"`
	MEV         int     `json:"mev"`
	MRV         int     `json:"mrv"`
	Verdict     string  `json:"verdict"`
}

type weeklyVolumeOut struct {
	From  string      `json:"from"`
	To    string      `json:"to"`
	Items []volumeOut `json:"items"`
}

type exerciseProgressIn struct {
	ExerciseID string `json:"exerciseId" jsonschema:"種目 ID。list_exercises で取得する"`
	From       string `json:"from,omitempty" jsonschema:"開始日 YYYY-MM-DD（JST）。省略時は42日前"`
	To         string `json:"to,omitempty" jsonschema:"終了日 YYYY-MM-DD（JST）。省略時は今日"`
}

type e1rmPointOut struct {
	Date     string  `json:"date"`
	BestE1RM float64 `json:"bestE1rm"`
	Sets     int     `json:"sets"`
}

type exerciseProgressOut struct {
	ExerciseID     string         `json:"exerciseId"`
	Points         []e1rmPointOut `json:"points"`
	SlopeKgPerWeek *float64       `json:"slopeKgPerWeek,omitempty"`
	Note           string         `json:"note,omitempty"`
}

type queryIn struct {
	SQL   string `json:"sql" jsonschema:"実行する SELECT 文。書き込みは拒否される"`
	Limit int    `json:"limit,omitempty" jsonschema:"最大行数。既定 100、上限 200"`
}

type queryOut struct {
	Columns   []string `json:"columns"`
	Rows      [][]any  `json:"rows"`
	Truncated bool     `json:"truncated"`
}

func registerAnalysisTools(s *mcp.Server, d Deps) {
	mcp.AddTool(s, &mcp.Tool{
		Name: "weekly_volume",
		Description: "部位別の週間セット数とトン数。MEV（最低有効ボリューム）/ MRV（最大回復可能ボリューム）と" +
			"突き合わせた判定を返す。MRV超の部位は削って MEV以下の部位に振り替える。総量を増やしてはいけない。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in weeklyVolumeIn) (*mcp.CallToolResult, weeklyVolumeOut, error) {
		from, to, err := dateRange(in.From, in.To, 7)
		if err != nil {
			return nil, weeklyVolumeOut{}, err
		}

		items, err := d.Analysis.WeeklyVolume(ctx, from, to)
		if err != nil {
			return nil, weeklyVolumeOut{}, err
		}

		out := weeklyVolumeOut{
			From:  from.Format("2006-01-02"),
			To:    to.Format("2006-01-02"),
			Items: make([]volumeOut, 0, len(items)),
		}
		for _, v := range items {
			verdict, r := analytics.JudgeVolume(v.MuscleGroup, v.Sets)
			out.Items = append(out.Items, volumeOut{
				MuscleGroup: string(v.MuscleGroup), Sets: v.Sets, TonnageKg: v.TonnageKg,
				MEV: r.MEV, MRV: r.MRV, Verdict: string(verdict),
			})
		}

		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "exercise_progress",
		Description: "種目の推定1RM（Epley + RIR補正）の推移と傾き。" +
			"RIR 未記録・限界12レップ超のセットは推定が崩れるため除外している。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in exerciseProgressIn) (*mcp.CallToolResult, exerciseProgressOut, error) {
		id, err := uuid.Parse(in.ExerciseID)
		if err != nil {
			return nil, exerciseProgressOut{}, fmt.Errorf("exerciseId が UUID ではない: %w", err)
		}
		from, to, err := dateRange(in.From, in.To, analytics.E1RMWindowDays)
		if err != nil {
			return nil, exerciseProgressOut{}, err
		}

		points, err := d.Analysis.ExerciseHistory(ctx, id, from, to)
		if err != nil {
			return nil, exerciseProgressOut{}, err
		}

		out := exerciseProgressOut{ExerciseID: in.ExerciseID, Points: make([]e1rmPointOut, 0, len(points))}
		xs := make([]float64, 0, len(points))
		ys := make([]float64, 0, len(points))
		for _, p := range points {
			out.Points = append(out.Points, e1rmPointOut{
				Date: p.Date.Format("2006-01-02"), BestE1RM: p.BestE1RM, Sets: p.Sets,
			})
			xs = append(xs, float64(p.Date.Unix())/float64(24*time.Hour/time.Second))
			ys = append(ys, p.BestE1RM)
		}

		if slope, ok := analytics.LinearSlope(xs, ys); ok {
			perWeek := slope * 7
			out.SlopeKgPerWeek = &perWeek
		} else {
			out.Note = "傾きを出すには計算できる日が2日以上必要。RIR を記録すると増える"
		}

		return nil, out, nil
	})

	mcp.AddTool(s, &mcp.Tool{
		Name: "query",
		Description: "読み取り専用の SQL を実行する。事前に定義できない分析はこれを使う。" +
			"テーブル: exercises / workout_sessions / workout_sets / templates / template_items。" +
			"すべて deleted_at による論理削除なので、生きている行だけ見るなら deleted_at is null を付ける。" +
			"日付は workout_sessions.date（JST の日付）。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in queryIn) (*mcp.CallToolResult, queryOut, error) {
		limit := in.Limit
		if limit <= 0 || limit > maxQueryRows {
			limit = maxQueryRows
		}

		res, err := d.Analysis.Query(ctx, in.SQL, limit)
		if err != nil {
			return nil, queryOut{}, err
		}

		return nil, queryOut{Columns: res.Columns, Rows: res.Rows, Truncated: res.Truncated}, nil
	})
}

// dateRange は from / to を解釈する。空なら today-defaultDays 〜 today。
func dateRange(from, to string, defaultDays int) (openapi_types.Date, openapi_types.Date, error) {
	// 「今日」は JST で決める（ADR-0013）。UTC だと日付が1日ずれる
	today := timeutil.Now().Truncate(24 * time.Hour)

	parse := func(s string, fallback time.Time) (openapi_types.Date, error) {
		if s == "" {
			return openapi_types.Date{Time: fallback}, nil
		}
		t, err := time.Parse("2006-01-02", s)
		if err != nil {
			return openapi_types.Date{}, fmt.Errorf("日付は YYYY-MM-DD で指定する: %q", s)
		}
		return openapi_types.Date{Time: t}, nil
	}

	t, err := parse(to, timeutil.Now())
	if err != nil {
		return openapi_types.Date{}, openapi_types.Date{}, err
	}
	f, err := parse(from, today.AddDate(0, 0, -defaultDays))
	if err != nil {
		return openapi_types.Date{}, openapi_types.Date{}, err
	}

	return f, t, nil
}
