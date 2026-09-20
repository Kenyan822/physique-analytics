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
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/vision"
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

// BodyRepository は日次記録と周囲長へのアクセス（要件 B-02 / B-03 / B-06）。
type BodyRepository interface {
	ListDaily(ctx context.Context, from, to *openapi_types.Date) ([]openapi.DailyMetrics, error)
	GetDaily(ctx context.Context, date openapi_types.Date) (openapi.DailyMetrics, error)
	PutDaily(ctx context.Context, in repository.DailyInput) (openapi.DailyMetrics, error)
	SoftDeleteDaily(ctx context.Context, date openapi_types.Date) error
	ListMeasurements(ctx context.Context, from, to *openapi_types.Date) ([]openapi.BodyMeasurement, error)
	PutMeasurement(ctx context.Context, in repository.MeasurementInput) (openapi.BodyMeasurement, error)
	LatestMeasurement(ctx context.Context) (openapi.BodyMeasurement, error)
}

// MealRepository は食事記録へのアクセス（要件 N-01 / N-02 / N-04）。
type MealRepository interface {
	List(ctx context.Context, from, to *openapi_types.Date) ([]openapi.Meal, error)
	Create(ctx context.Context, in repository.MealInput) (openapi.Meal, error)
	Update(ctx context.Context, id uuid.UUID, in repository.MealInput) (openapi.Meal, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Suggestions(ctx context.Context, query string, limit int) ([]openapi.MealSuggestion, error)
	Copy(ctx context.Context, from, to openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error)
}

// FoodItemRepository は食品マスタへのアクセス（要件 N-02 / ADR-0017）。
//
// **nil でもよい。** 未設定なら食品マスタだけが使えない
type FoodItemRepository interface {
	List(ctx context.Context, q string) ([]openapi.FoodItem, error)
	Get(ctx context.Context, id uuid.UUID) (openapi.FoodItem, error)
	Create(ctx context.Context, in openapi.FoodItemInput) (openapi.FoodItem, error)
	Update(ctx context.Context, id uuid.UUID, in openapi.FoodItemInput) (openapi.FoodItem, error)
	Delete(ctx context.Context, id uuid.UUID) error
	MarkUsed(ctx context.Context, id uuid.UUID) error
}

// ManualTargetsRepository は手で決めた摂取目標へのアクセス（要件 N-05）。
//
// **nil でもよい。** 未設定なら自動計算（A-02）だけが使われる
type ManualTargetsRepository interface {
	Get(ctx context.Context) (*openapi.ManualTargets, error)
	Put(ctx context.Context, in openapi.ManualTargets) (openapi.ManualTargets, error)
	Delete(ctx context.Context) error
}

// PlanRepository は計画の設定へのアクセス（要件 P-01 / P-05）。
type PlanRepository interface {
	Get(ctx context.Context) (openapi.Plan, error)
	Put(ctx context.Context, in openapi.PlanInput) (openapi.Plan, error)
	ListBlocks(ctx context.Context) ([]openapi.PlanBlock, error)
	PutBlocks(ctx context.Context, blocks []openapi.PlanBlock) ([]openapi.PlanBlock, error)
}

// SeriesRepository は日次記録の系列（要件 N-05 の目標計算に使う）。
type SeriesRepository interface {
	DailySeries(ctx context.Context, from, to openapi_types.Date) ([]analytics.DailyPoint, error)
}

// MealSetRepository は食事セットへのアクセス（要件 N-03）。
type MealSetRepository interface {
	List(ctx context.Context) ([]openapi.MealSet, error)
	Create(ctx context.Context, in openapi.MealSetInput) (openapi.MealSet, error)
	Update(ctx context.Context, id uuid.UUID, in openapi.MealSetInput) (openapi.MealSet, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
	Apply(ctx context.Context, id uuid.UUID, date openapi_types.Date, slot *openapi.MealSlot) ([]openapi.Meal, error)
}

// ContestRepository は大会へのアクセス（要件 P-04）。
type ContestRepository interface {
	List(ctx context.Context) ([]openapi.Contest, error)
	Next(ctx context.Context, asof openapi_types.Date) (openapi.Contest, error)
	Create(ctx context.Context, in openapi.ContestInput) (openapi.Contest, error)
	Update(ctx context.Context, id uuid.UUID, in openapi.ContestInput) (openapi.Contest, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// BloodTestRepository は血液検査へのアクセス（要件 B-08）。
type BloodTestRepository interface {
	List(ctx context.Context) ([]openapi.BloodTest, error)
	Create(ctx context.Context, in openapi.BloodTestInput) (openapi.BloodTest, error)
	Update(ctx context.Context, id uuid.UUID, in openapi.BloodTestInput) (openapi.BloodTest, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// FoodEstimator は写真から PFC を推定する（要件 N-06）。
//
// インターフェースにしてあるのは、**テストで実 API を叩かないため**。
// 課金が発生する経路をテストが通ることはない。
type FoodEstimator interface {
	Enabled() bool
	Estimate(ctx context.Context, req vision.Request) (vision.Estimate, error)
}

// PhotoRepository は身体写真のメタデータへのアクセス（要件 B-04 / B-05 / B-07）。
type PhotoRepository interface {
	List(ctx context.Context, from, to *openapi_types.Date) ([]repository.PhotoRow, error)
	Guide(ctx context.Context, before openapi_types.Date) ([]repository.PhotoRow, error)
	Create(ctx context.Context, in repository.PhotoInput) (repository.PhotoRow, error)
	SoftDelete(ctx context.Context, id uuid.UUID) error
}

// BlobStore は写真の置き場。**画像は API を経由させない**（ADR-0008）。
type BlobStore interface {
	Enabled() bool
	PresignGet(key string, expiry time.Duration, now time.Time) (string, error)
	PresignPut(key string, expiry time.Duration, now time.Time) (string, error)
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
	body      BodyRepository
	meals     MealRepository
	plan      PlanRepository
	series    SeriesRepository
	mealSets  MealSetRepository
	contests  ContestRepository
	blood     BloodTestRepository
	// estimator は nil でもよい。未設定なら推定だけが使えない
	estimator FoodEstimator
	photos    PhotoRepository
	// blobs も nil でよい。未設定なら写真だけが使えない
	blobs BlobStore
	// manualTargets は任意。**引数に足さずセッターにしてある** ——
	// New は既に16引数で、呼ぶ側19箇所すべてを触る手間に見合わない
	manualTargets ManualTargetsRepository
	// foodItems も任意。未設定なら食品マスタだけが使えない
	foodItems FoodItemRepository
}

// WithFoodItems は食品マスタの置き場所を差す（要件 N-02）。
func (s *Server) WithFoodItems(r FoodItemRepository) *Server {
	s.foodItems = r

	return s
}

// WithManualTargets は手で決めた摂取目標の置き場所を差す（要件 N-05）。
func (s *Server) WithManualTargets(r ManualTargetsRepository) *Server {
	s.manualTargets = r

	return s
}

// New は Server を作る。
func New(
	db Pinger,
	exercises ExerciseRepository,
	workouts WorkoutRepository,
	templates TemplateRepository,
	sync SyncRepository,
	transfer TransferRepository,
	body BodyRepository,
	meals MealRepository,
	plan PlanRepository,
	series SeriesRepository,
	mealSets MealSetRepository,
	contests ContestRepository,
	blood BloodTestRepository,
	estimator FoodEstimator,
	photos PhotoRepository,
	blobs BlobStore,
) *Server {
	return &Server{
		db: db, exercises: exercises, workouts: workouts,
		templates: templates, sync: sync, transfer: transfer,
		body: body, meals: meals, plan: plan, series: series,
		mealSets: mealSets, contests: contests, blood: blood, estimator: estimator,
		photos: photos, blobs: blobs,
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
