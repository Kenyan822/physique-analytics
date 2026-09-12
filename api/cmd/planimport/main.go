// Command planimport は config.json の計画設定を DB に取り込む。
//
// 設定の正は DB に移した（要件 P-05）。**一度きりの移行用**で、
// 以降は Web の設定画面か PUT /v1/plan から変える。
//
//	PHYSIQUE_CONFIG=private/config.json DATABASE_URL=... go run ./cmd/planimport
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/config"
	"github.com/Kenyan822/physique-analytics/api/internal/database"
	"github.com/Kenyan822/physique-analytics/api/internal/plan"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if err := run(); err != nil {
		slog.Error("取り込みに失敗", slog.Any("error", err))
		os.Exit(1)
	}
}

func run() error {
	path, ok := os.LookupEnv("PHYSIQUE_CONFIG")
	if !ok || path == "" {
		return errors.New("PHYSIQUE_CONFIG に config.json のパスを設定する")
	}

	src, err := plan.Load(path)
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cfg, err := config.LoadForMCP(os.LookupEnv)
	if err != nil {
		return err
	}
	pool, err := database.Open(ctx, database.Config{DSN: cfg.DatabaseURL, MaxConns: cfg.DatabaseMaxConns})
	if err != nil {
		return err
	}
	defer pool.Close()

	in, err := toPlanInput(src)
	if err != nil {
		return err
	}

	saved, err := repository.NewPlan(pool.DB()).Put(ctx, in)
	if err != nil {
		return err
	}

	slog.Info("取り込んだ",
		slog.Int("フェーズ", len(saved.Phases)),
		slog.Int("部位", len(saved.VolumeRanges)))

	return nil
}

func toPlanInput(src plan.Plan) (openapi.PlanInput, error) {
	out := openapi.PlanInput{
		Phases:       make([]openapi.PlanPhase, 0, len(src.Phases)),
		VolumeRanges: make([]openapi.VolumeRange, 0, len(src.Volume)),
		Nutrition: openapi.NutritionSettings{
			Cut:                macroRatio(src.Nutrition.Cut),
			DeepCut:            macroRatio(src.Nutrition.DeepCut),
			Bulk:               macroRatio(src.Nutrition.Bulk),
			DeepCutBfThreshold: float32(src.Nutrition.DeepCutBfThreshold),
			CarbMinG:           int(src.Nutrition.CarbMinG),
		},
	}
	if src.HeightCm > 0 {
		h := float32(src.HeightCm)
		out.HeightCm = &h
	}

	for _, ph := range src.Phases {
		from, err := time.Parse(time.DateOnly, ph.From)
		if err != nil {
			return openapi.PlanInput{}, fmt.Errorf("フェーズ %q の開始日: %w", ph.Name, err)
		}
		to, err := time.Parse(time.DateOnly, ph.To)
		if err != nil {
			return openapi.PlanInput{}, fmt.Errorf("フェーズ %q の終了日: %w", ph.Name, err)
		}
		out.Phases = append(out.Phases, openapi.PlanPhase{
			Name:          ph.Name,
			StartsOn:      openapi_types.Date{Time: from},
			EndsOn:        openapi_types.Date{Time: to},
			GoalKgPerWeek: float32(ph.GoalKgPerWeek),
		})
	}

	for muscle, r := range src.Volume {
		// "default" は部位ではないので飛ばす。DB 側は部位ごとに持つ
		mg := openapi.MuscleGroup(muscle)
		if !mg.Valid() {
			continue
		}
		out.VolumeRanges = append(out.VolumeRanges, openapi.VolumeRange{
			MuscleGroup: mg, Mev: r.Min, Mrv: r.Max,
		})
	}

	return out, nil
}

func macroRatio(m plan.Macros) openapi.MacroRatio {
	return openapi.MacroRatio{
		ProteinGPerKg: float32(m.ProteinGPerKg),
		FatGPerKg:     float32(m.FatGPerKg),
	}
}
