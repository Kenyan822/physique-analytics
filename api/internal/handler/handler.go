// Package handler は openapi.yaml から生成されたインターフェースの実装を持つ。
package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/google/uuid"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
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
}

// Server は openapi.StrictServerInterface の実装。
// 未実装の操作は埋め込んだ Unimplemented が 501 で受ける。
type Server struct {
	Unimplemented

	db        Pinger
	exercises ExerciseRepository
}

// New は Server を作る。
func New(db Pinger, exercises ExerciseRepository) *Server {
	return &Server{db: db, exercises: exercises}
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
	var ni notImplementedError
	if errors.As(err, &ni) {
		writeProblem(w, http.StatusNotImplemented, "未実装", ni.op+" はまだ実装されていない")
		return
	}

	// 内部エラーの中身はログにだけ残す。接続先やクエリが応答に混ざるのを避ける
	slog.ErrorContext(r.Context(), "ハンドラがエラーを返した",
		slog.String("method", r.Method), slog.String("path", r.URL.Path), slog.Any("error", err))
	writeProblem(w, http.StatusInternalServerError, "サーバ内部エラー", "")
}

// writeProblem は RFC 7807 の形式でエラーを返す。
func writeProblem(w http.ResponseWriter, status int, title, detail string) {
	p := openapi.Problem{
		Type:   "about:blank",
		Title:  title,
		Status: status,
	}
	if detail != "" {
		p.Detail = &detail
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(p); err != nil {
		slog.Error("problem の書き込みに失敗", slog.Any("error", err))
	}
}

type notImplementedError struct{ op string }

func (e notImplementedError) Error() string { return e.op + " は未実装" }

func notImplemented(op string) error { return notImplementedError{op: op} }
