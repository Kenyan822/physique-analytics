// Command mcp は Claude Code から分析するための MCP サーバ（ADR-0010）。
//
// stdio で話す。手元でしか動かさないので認証は無い。
//
//	claude mcp add physique -- /path/to/mcp
package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/Kenyan822/physique-analytics/api/internal/config"
	"github.com/Kenyan822/physique-analytics/api/internal/database"
	"github.com/Kenyan822/physique-analytics/api/internal/mcpserver"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func main() {
	// stdout は MCP のプロトコルが使う。ログは stderr へ出す
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("起動に失敗", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// MCP は手元から DB を直接読む。API を経由しないので認証は要らない
	cfg, err := config.LoadForMCP(os.LookupEnv)
	if err != nil {
		return err
	}

	pool, err := database.Open(ctx, database.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DatabaseMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	srv := mcpserver.New(mcpserver.Deps{
		Exercises: repository.NewExercise(pool.DB()),
		Workouts:  repository.NewWorkout(pool.DB()),
		Analysis:  repository.NewAnalysis(pool.DB()),
		// 計画の設定は DB を正とする（要件 P-05）。
		// config.json から移すには `go run ./cmd/planimport` を使う
		Plan:     repository.NewPlan(pool.DB()),
		Contests: repository.NewContest(pool.DB()),
	})

	slog.Info("MCP サーバを開始")

	return srv.Run(ctx, &mcp.StdioTransport{})
}
