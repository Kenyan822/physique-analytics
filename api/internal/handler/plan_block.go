package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
)

// ListPlanBlocks は計画のブロックを返す（要件 P-02）。
func (s *Server) ListPlanBlocks(ctx context.Context, _ openapi.ListPlanBlocksRequestObject) (openapi.ListPlanBlocksResponseObject, error) {
	items, err := s.plan.ListBlocks(ctx)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.PlanBlock{}
	}

	return openapi.ListPlanBlocks200JSONResponse{Items: items}, nil
}

// PutPlanBlocks はブロックをまるごと置き換える。
func (s *Server) PutPlanBlocks(ctx context.Context, req openapi.PutPlanBlocksRequestObject) (openapi.PutPlanBlocksResponseObject, error) {
	if req.Body == nil {
		return putBlocksFailed("body", "リクエストボディが無い"), nil
	}
	for i, b := range req.Body.Items {
		if msg := validatePlanBlock(b); msg != "" {
			return putBlocksFailed(fmt.Sprintf("items[%d]", i), msg), nil
		}
	}

	items, err := s.plan.PutBlocks(ctx, req.Body.Items)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.PlanBlock{}
	}

	return openapi.PutPlanBlocks200JSONResponse{Items: items}, nil
}

// GetMonthlyTargets は月次目標を計算して返す（要件 P-02 / P-03）。
//
// **月ごとの表を持たない。** 起点とブロックから毎回計算する。
// 保存すると「引き直したつもりで古い表が残っている」が起きる。
func (s *Server) GetMonthlyTargets(ctx context.Context, req openapi.GetMonthlyTargetsRequestObject) (openapi.GetMonthlyTargetsResponseObject, error) {
	plan, err := s.plan.Get(ctx)
	if err != nil {
		return nil, err
	}
	blocks, err := s.plan.ListBlocks(ctx)
	if err != nil {
		return nil, err
	}
	if len(blocks) == 0 {
		return monthlyTargetsFailed("blocks",
			"計画のブロックが登録されていない。設定画面か PUT /v1/plan/blocks で登録する"), nil
	}
	if plan.HeightCm == nil {
		return monthlyTargetsFailed("heightCm", "身長が未設定。FFMI を出せない"), nil
	}

	base, source, note, err := s.resolveBaseline(ctx, plan, req.Params.Baseline)
	if err != nil {
		return nil, err
	}
	if base == nil {
		return monthlyTargetsFailed("baseline", note), nil
	}
	base.HeightCm = float64(*plan.HeightCm)

	month := "2026-01"
	if plan.BaselineMonth != nil && *plan.BaselineMonth != "" {
		month = *plan.BaselineMonth
	}

	out := openapi.MonthlyTargets{
		Baseline: struct {
			BodyfatPct float32                              `json:"bodyfatPct"`
			Month      string                               `json:"month"`
			Source     openapi.MonthlyTargetsBaselineSource `json:"source"`
			WeightKg   float32                              `json:"weightKg"`
		}{
			Source:     source,
			WeightKg:   float32(base.WeightKg),
			BodyfatPct: float32(base.BodyfatPct),
			Month:      month,
		},
		Items: make([]openapi.MonthlyTarget, 0, 40),
	}
	if note != "" {
		out.Note = &note
	}

	for _, t := range analytics.MonthlyTargets(*base, repository.ToAnalyticsBlocks(blocks), month) {
		out.Items = append(out.Items, openapi.MonthlyTarget{
			Month:      t.Month,
			Phase:      t.Phase,
			LbmKg:      float32(t.LbmKg),
			BodyfatPct: float32(t.BodyfatPct),
			WeightKg:   float32(t.WeightKg),
			Ffmi:       float32(t.Ffmi),
		})
	}

	return openapi.GetMonthlyTargets200JSONResponse(out), nil
}

// resolveBaseline は起点を決める。
//
// measured を指定されたら直近の実測（7日平均）を使う（要件 P-03）。
// **実測が無いときに設定値へ黙って落ちない。** どちらを見ているか
// 分からないまま数字を読むと判断を誤る。
func (s *Server) resolveBaseline(
	ctx context.Context,
	plan openapi.Plan,
	param *openapi.GetMonthlyTargetsParamsBaseline,
) (*analytics.Baseline, openapi.MonthlyTargetsBaselineSource, string, error) {
	if param != nil && *param == openapi.GetMonthlyTargetsParamsBaselineMeasured {
		sum, err := weekly.Build(ctx, weekly.Deps{Plan: s.plan, Series: s.series}, timeutil.Now().Truncate(24*time.Hour))
		if err != nil {
			return nil, "", "", err
		}
		if sum.WeightKg7dAvg == nil || sum.BodyfatPct7dAvg == nil {
			return nil, "", "直近7日の体重または体脂肪率が無いため、実測から引き直せない", nil
		}

		return &analytics.Baseline{
			WeightKg:   *sum.WeightKg7dAvg,
			BodyfatPct: *sum.BodyfatPct7dAvg,
		}, openapi.MonthlyTargetsBaselineSourceMeasured, "", nil
	}

	if plan.BaselineWeightKg == nil || plan.BaselineBodyfatPct == nil {
		return nil, "", "起点が未設定。設定画面で体重と体脂肪率を入れる", nil
	}

	return &analytics.Baseline{
		WeightKg:   float64(*plan.BaselineWeightKg),
		BodyfatPct: float64(*plan.BaselineBodyfatPct),
	}, openapi.MonthlyTargetsBaselineSourceConfigured, "", nil
}

func validatePlanBlock(b openapi.PlanBlock) string {
	if strings.TrimSpace(b.Name) == "" {
		return "名前が空"
	}
	if b.Months < 1 || b.Months > 60 {
		return "月数は 1〜60 にする"
	}
	if b.LbmDeltaKgPerMonth < -2 || b.LbmDeltaKgPerMonth > 2 {
		return "LBM の増減は ±2kg/月 以内にする"
	}
	if b.BodyfatPctEnd <= 0 || b.BodyfatPctEnd >= 60 {
		return "終了時点の体脂肪率は 0 より大きく 60 未満にする"
	}

	return ""
}

func putBlocksFailed(field, message string) openapi.PutPlanBlocks422ApplicationProblemPlusJSONResponse {
	return openapi.PutPlanBlocks422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func monthlyTargetsFailed(field, message string) openapi.GetMonthlyTargets422ApplicationProblemPlusJSONResponse {
	return openapi.GetMonthlyTargets422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
