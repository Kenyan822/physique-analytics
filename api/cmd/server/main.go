// Command server は physique-analytics の API サーバ。
package main

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/auth"
	"github.com/Kenyan822/physique-analytics/api/internal/blobstore"
	"github.com/Kenyan822/physique-analytics/api/internal/config"
	"github.com/Kenyan822/physique-analytics/api/internal/database"
	"github.com/Kenyan822/physique-analytics/api/internal/handler"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/vision"
)

func main() {
	// Cloud Logging は JSON のログを構造化して取り込む
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

	if err := run(); err != nil {
		slog.Error("起動に失敗", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	// SIGTERM は Cloud Run がインスタンスを止めるときに送ってくる
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return err
	}

	pool, err := database.Open(ctx, database.Config{
		DSN:      cfg.DatabaseURL,
		MaxConns: cfg.DatabaseMaxConns,
	})
	if err != nil {
		return err
	}
	defer pool.Close()

	// NewRouter は http.Handler を返す。認証ミドルウェアで包み直すので再代入する
	h := handler.NewRouter(handler.New(
		pool,
		repository.NewExercise(pool.DB()),
		repository.NewWorkout(pool.DB()),
		repository.NewTemplate(pool.DB()),
		repository.NewSync(pool.DB()),
		repository.NewTransfer(pool.DB()),
		repository.NewBody(pool.DB()),
		repository.NewMeal(pool.DB()),
		repository.NewPlan(pool.DB()),
		repository.NewAnalysis(pool.DB()),
		repository.NewMealSet(pool.DB()),
		repository.NewContest(pool.DB()),
		repository.NewBloodTest(pool.DB()),
		// 推定は任意。API キーが無ければ 503 を返すだけで、課金は発生しない
		vision.NewClient(vision.Config{
			APIKey:   cfg.VisionAPIKey,
			Model:    cfg.VisionModel,
			Endpoint: cfg.VisionEndpoint,
		}, nil),
		repository.NewPhoto(pool.DB()),
		// 写真も任意。未設定なら 503 を返すだけ
		blobstore.NewR2(blobstore.Config{
			AccountID:       cfg.R2AccountID,
			Bucket:          cfg.R2Bucket,
			AccessKeyID:     cfg.R2AccessKeyID,
			SecretAccessKey: cfg.R2SecretAccessKey,
		}),
	))

	if cfg.AuthDisabled {
		// ローカル開発専用。本番でこのログが出ていたら設定ミス
		slog.Warn("認証が無効になっている。ローカル開発以外では使わない")
	} else {
		// /health は openapi.yaml で security: [] になっている。
		// keepalive と Cloud Run のスモークテストが叩くため
		v := auth.NewVerifier(cfg.SupabaseJWKSURL, cfg.AllowedUserIDs...)
		if !v.Allowlisted() {
			// **JWT の検証だけでは所有者を区別できない。** Supabase の
			// サインアップが開いていれば、登録した人は誰でも全データを読める（#51）
			slog.Warn("ALLOWED_USER_IDS が未設定。Supabase に登録した利用者は全員データを読める",
				"対処", "自分の auth.users.id を ALLOWED_USER_IDS に設定する")
		}
		h = auth.Middleware(v, "/health")(h)
	}

	srv := &http.Server{
		Addr:    net.JoinHostPort("", strconv.Itoa(cfg.Port)),
		Handler: h,
		// ヘッダを送り切らない接続にワーカーを占有させない
		ReadHeaderTimeout: 10 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("起動", slog.Int("port", cfg.Port))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		slog.Info("終了シグナルを受け取った。処理中のリクエストを待つ")
	}

	// Cloud Run は SIGTERM の 10 秒後に強制終了する。それより短く切り上げる
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
