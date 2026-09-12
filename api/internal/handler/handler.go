// Package handler は openapi.yaml から生成されたインターフェースの実装を持つ。
package handler

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// Pinger は疎通確認できるデータストア。ヘルスチェックのテストで実 DB を立てないために挟んでいる。
type Pinger interface {
	Ping(ctx context.Context) error
}

// ExerciseRepository は種目マスタへのアクセス。
// ハンドラが必要とする操作だけを並べている（実装は internal/repository）。
type ExerciseRepository interface {
	List(ctx context.Context, f repository.ExerciseFilter) ([]openapi.Exercise, error)
	Get(ctx context.Context, id uuid.UUID) (openapi.Exercise, error)
	Create(ctx context.Context, in repository.ExerciseInput) (openapi.Exercise, error)
	Update(ctx context.Context, id uuid.UUID, in repository.ExerciseInput) (openapi.Exercise, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// WorkoutRepository はトレーニング記録へのアクセス。
type WorkoutRepository interface {
	ListSessions(ctx context.Context, f repository.SessionFilter) ([]openapi.WorkoutSession, error)
	CreateSessionIdempotent(ctx context.Context, in repository.SessionInput) (openapi.WorkoutSession, bool, error)
	GetSession(ctx context.Context, id uuid.UUID) (openapi.WorkoutSession, error)
	UpdateSession(ctx context.Context, id uuid.UUID, up repository.SessionUpdate) (openapi.WorkoutSession, error)
	DeleteSession(ctx context.Context, id uuid.UUID) error
	CreateSet(ctx context.Context, sessionID uuid.UUID, in repository.SetInput) (openapi.WorkoutSet, error)
	UpdateSet(ctx context.Context, id uuid.UUID, in repository.SetInput) (openapi.WorkoutSet, error)
	DeleteSet(ctx context.Context, id uuid.UUID) error
	LastPerformance(ctx context.Context, exerciseID uuid.UUID) (repository.LastPerformanceResult, error)
}

// TemplateRepository はトレーニングテンプレートへのアクセス。
type TemplateRepository interface {
	List(ctx context.Context) ([]openapi.Template, error)
	Get(ctx context.Context, id uuid.UUID) (openapi.Template, error)
	Create(ctx context.Context, in repository.TemplateInput) (openapi.Template, error)
	Update(ctx context.Context, id uuid.UUID, in repository.TemplateInput) (openapi.Template, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// SyncRepository はオフライン同期（ADR-0014）。
type SyncRepository interface {
	Pull(ctx context.Context, since time.Time) (repository.PullResult, error)
	Push(ctx context.Context, in repository.PushInput) (repository.PushResult, error)
}

// TransferRepository は CSV の入出力（要件 I-01）。
type TransferRepository interface {
	ExportDaily(ctx context.Context, from, to *openapi_types.Date) ([]csvio.DailyRow, error)
	ExportWorkouts(ctx context.Context, from, to *openapi_types.Date) ([]csvio.WorkoutRow, error)
	ExportMeasures(ctx context.Context, from, to *openapi_types.Date) ([]csvio.MeasureRow, error)
	ImportDaily(ctx context.Context, rows []csvio.DailyRow, on repository.OnDuplicate) (repository.ImportResult, error)
	ImportWorkouts(ctx context.Context, rows []csvio.WorkoutRow, on repository.OnDuplicate) (repository.ImportResult, error)
	ImportMeasures(ctx context.Context, rows []csvio.MeasureRow, on repository.OnDuplicate) (repository.ImportResult, error)
}

// Server は openapi.StrictServerInterface の実装。
//
// **openapi.yaml の全操作を実装している。** 仕様に操作を足すと
// StrictServerInterface を満たさなくなりビルドが落ちるので、
// 「仕様に足したのにサーバ側が追随していない」はコンパイル時に分かる
// （下の var _ = ... がそれを強制する）。
type Server struct {
	db        Pinger
	exercises ExerciseRepository
	workouts  WorkoutRepository
	templates TemplateRepository
	sync      SyncRepository
	transfer  TransferRepository
}

// New は Server を作る。
func New(
	db Pinger,
	exercises ExerciseRepository,
	workouts WorkoutRepository,
	templates TemplateRepository,
	sync SyncRepository,
	transfer TransferRepository,
) *Server {
	return &Server{
		db: db, exercises: exercises, workouts: workouts,
		templates: templates, sync: sync, transfer: transfer,
	}
}

// 埋め込みだけでは満たせていない場合にコンパイルで落とす
var _ openapi.StrictServerInterface = (*Server)(nil)

// NewRouter は openapi.yaml から生成されたルーティングに s を接続する。
//
// 生成されたデフォルトのエラーハンドラは text/plain の http.Error を返し、
// エラー形式を problem+json に統一するという仕様（openapi.yaml の Problem）から外れる。
// そのため両方のエラーハンドラを差し替えている。
func NewRouter(s openapi.StrictServerInterface) http.Handler {
	strict := openapi.NewStrictHandlerWithOptions(s, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			// リクエストのパース失敗。原因は送った側にあるので返してよい
			writeProblem(w, http.StatusBadRequest, "リクエストが不正", err.Error())
		},
		ResponseErrorHandlerFunc: handleResponseError,
	})

	return openapi.HandlerWithOptions(strict, openapi.StdHTTPServerOptions{
		BaseRouter: http.NewServeMux(),
		ErrorHandlerFunc: func(w http.ResponseWriter, _ *http.Request, err error) {
			writeProblem(w, http.StatusBadRequest, "リクエストが不正", err.Error())
		},
	})
}

func handleResponseError(w http.ResponseWriter, r *http.Request, err error) {
	// 内部エラーの中身はログにだけ残す。接続先やクエリが応答に混ざるのを避ける
	slog.ErrorContext(r.Context(), "ハンドラがエラーを返した",
		slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
	writeProblem(w, http.StatusInternalServerError, "サーバ内部エラー", "")
}

// problem は RFC 7807 の Problem を組み立てる。
func problem(status int, title, detail string) openapi.Problem {
	p := openapi.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
	}
	if detail != "" {
		p.Detail = &detail
	}

	return p
}

// writeProblem は RFC 7807 の形式でエラーを返す。
func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	p := problem(status, title, detail)

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Error("problem の書き込みに失敗", slog.Any("error", err))
	}
}
