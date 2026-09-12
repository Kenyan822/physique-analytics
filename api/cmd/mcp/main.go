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
	"github.com/Kenyan822/physique-analytics/api/internal/plan"
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

	deps := mcpserver.Deps{
		Exercises: repository.NewExercise(pool.DB()),
		Workouts:  repository.NewWorkout(pool.DB()),
		Analysis:  repository.NewAnalysis(pool.DB()),
	}

	// 計画の設定（目標ペース・PFC 係数）。個人データなので任意にしてある。
	// 無くても他のツールは動き、weekly_actions だけが使えない
	if path, ok := os.LookupEnv("PHYSIQUE_CONFIG"); ok && path != "" {
		p, err := plan.Load(path)
		if err != nil {
			return err
		}
		deps.Plan = &p
	} else {
		slog.Warn("PHYSIQUE_CONFIG が未設定。weekly_actions は使えない")
	}

	srv := mcpserver.New(deps)

	slog.Info("MCP サーバを開始")

	return srv.Run(ctx, &mcp.StdioTransport{})
}
