// 内部テストにしているのは、weeklyActions を MCP のセッションを張らずに
// 呼ぶため。ツール登録の薄い層のためだけにクライアントを立てる価値は無い。
package mcpserver

import (
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"

	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/testdb"
)

// fixture はトランザクション内で動く Deps と、記録を入れるための Body を返す。
//
// withPlan のときはフェーズを1つ入れる。**公開用の設定例と同じ値**を使い、
// private/config.json はテストから参照しない（ADR-0002）。
func fixture(t *testing.T, withPlan bool) (Deps, *repository.Body) {
	t.Helper()

	tx := testdb.Begin(t)
	d := Deps{
		Exercises: repository.NewExercise(tx),
		Workouts:  repository.NewWorkout(tx),
		Analysis:  repository.NewAnalysis(tx),
		Plan:      repository.NewPlan(tx),
	}
	if !withPlan {
		// 手元の DB には取り込み済みのフェーズがコミットされている。
		// トランザクション内で消せば、他のテストには影響しない
		if _, err := tx.Exec(t.Context(), "delete from plan_phases"); err != nil {
			t.Fatalf("フェーズを消せない: %v", err)
		}
	}

	if withPlan {
		in := openapi.PlanInput{
			HeightCm: f32(175),
			Phases: []openapi.PlanPhase{{
				Name:          "P1-A カット1.0%",
				StartsOn:      openapi_types.Date{Time: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)},
				EndsOn:        openapi_types.Date{Time: time.Date(2029, 9, 6, 0, 0, 0, 0, time.UTC)},
				GoalKgPerWeek: -0.76,
			}},
			Nutrition: openapi.NutritionSettings{
				Cut:                openapi.MacroRatio{ProteinGPerKg: 2.4, FatGPerKg: 0.85},
				DeepCut:            openapi.MacroRatio{ProteinGPerKg: 2.6, FatGPerKg: 0.85},
				Bulk:               openapi.MacroRatio{ProteinGPerKg: 2.2, FatGPerKg: 1.0},
				DeepCutBfThreshold: 13.0,
				CarbMinG:           200,
			},
			VolumeRanges: []openapi.VolumeRange{},
		}
		if _, err := d.Plan.Put(t.Context(), in); err != nil {
			t.Fatalf("計画の設定を入れられない: %v", err)
		}
	}

	return d, repository.NewBody(tx)
}

func TestWeeklyActions_フェーズが無ければエラー(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, false)

	_, err := weeklyActions(t.Context(), d, weeklyActionsIn{AsOf: "2028-06-15"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	// エラーには直し方を書く。MCP のエラーは Claude が読んで利用者に伝える
	if !strings.Contains(err.Error(), "設定画面") {
		t.Errorf("err = %v, want 直し方を含む", err)
	}
}

func TestWeeklyActions_日付の形式(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	if _, err := weeklyActions(t.Context(), d, weeklyActionsIn{AsOf: "2026/09/30"}); err == nil {
		t.Error("エラーにならない")
	}
}

func TestWeeklyActions_記録が無ければ記録を促す(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	got, err := weeklyActions(t.Context(), d, weeklyActionsIn{AsOf: "2028-06-15"})
	if err != nil {
		t.Fatalf("weeklyActions: %v", err)
	}

	if got.TDEEKcal != nil {
		t.Errorf("TDEEKcal = %v, want nil（記録が無い）", got.TDEEKcal)
	}
	if !strings.Contains(got.Note, "TDEE") {
		t.Errorf("Note = %q, want TDEE を推定できない旨", got.Note)
	}
	if len(got.Actions) == 0 {
		t.Fatal("Actions が空")
	}
	// 直近7日が丸ごと未記録なので、記録漏れが先頭に来る
	if got.Actions[0].Kind != "missing_records" {
		t.Errorf("先頭 = %q, want missing_records", got.Actions[0].Kind)
	}
}

func TestWeeklyActions_記録からTDEEと推奨摂取を出す(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	d, body := fixture(t, true)

	// 2026-10-31 を基準に21日分。体重は1日 -0.06kg、摂取は 2100kcal 一定
	// 手元の DB にはサンプルが 2026-09〜10 に入っている。
	// 重ならない日付を選ぶ（計画の期間内であることは必要）
	asof := time.Date(2028, 6, 30, 0, 0, 0, 0, time.UTC)
	for i := range 21 {
		date := asof.AddDate(0, 0, -i)
		if _, err := body.PutDaily(ctx, repository.DailyInput{
			Date:     openapi_types.Date{Time: date},
			WeightKg: f32(75.0 + 0.06*float64(i)),
			Kcal:     ip(2100),
			Steps:    ip(9000),
		}); err != nil {
			t.Fatalf("PutDaily: %v", err)
		}
	}

	got, err := weeklyActions(ctx, d, weeklyActionsIn{AsOf: "2028-06-30"})
	if err != nil {
		t.Fatalf("weeklyActions: %v", err)
	}

	if got.TDEEKcal == nil {
		t.Fatalf("TDEEKcal = nil, want 推定できる。Note = %q", got.Note)
	}
	// 2100 - (-0.42 × 7700 / 7) = 2562
	if *got.TDEEKcal < 2550 || *got.TDEEKcal > 2575 {
		t.Errorf("TDEEKcal = %.0f, want 2562 前後", *got.TDEEKcal)
	}
	if got.IntakeKcal == nil || got.ProteinG == nil || got.CarbG == nil {
		t.Error("推奨摂取と PFC が出ていない")
	}
	// 計画の期間内なのでフェーズ名が付く
	if got.Phase == "" {
		t.Error("Phase が空")
	}
	// 目標どおりに落ちているので停滞にはならない
	for _, a := range got.Actions {
		if a.Kind == "weight_plateau" {
			t.Errorf("停滞を検知している: %s", a.Text)
		}
	}
}

func TestWeeklyActions_計画の期間外(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	got, err := weeklyActions(t.Context(), d, weeklyActionsIn{AsOf: "2020-01-01"})
	if err != nil {
		t.Fatalf("weeklyActions: %v", err)
	}

	if got.Phase != "" {
		t.Errorf("Phase = %q, want 空", got.Phase)
	}
	if !strings.Contains(got.Note, "期間外") {
		t.Errorf("Note = %q, want 期間外である旨", got.Note)
	}
}

func f32(v float64) *float32 { f := float32(v); return &f }
func ip(v int) *int          { return &v }

func TestWeeklyReport_Markdownを返す(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	got, err := weeklyReport(t.Context(), d, weeklyReportIn{AsOf: "2028-06-15"})
	if err != nil {
		t.Fatalf("weeklyReport: %v", err)
	}

	if got.AsOf != "2028-06-15" {
		t.Errorf("AsOf = %q", got.AsOf)
	}
	for _, want := range []string{"# 週次レポート", "## 1. 体重トレンド", "## 8. 今週のアクション"} {
		if !strings.Contains(got.Markdown, want) {
			t.Errorf("%q が無い\n---\n%s", want, got.Markdown)
		}
	}
}

func TestWeeklyReport_フェーズが無ければエラー(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, false)

	if _, err := weeklyReport(t.Context(), d, weeklyReportIn{AsOf: "2028-06-15"}); err == nil {
		t.Error("エラーにならない")
	}
}

func TestCorrelations_サンプルが足りなければ示す(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	got, err := correlations(t.Context(), d, correlationsIn{Days: 365})
	if err != nil {
		t.Fatalf("correlations: %v", err)
	}

	if len(got.Items) != 5 {
		t.Fatalf("項目 = %d, want 5", len(got.Items))
	}
	// 手元のサンプルは55日ぶんしかない。**足りないことが分かる形で返す**
	for _, i := range got.Items {
		if i.Enough {
			t.Errorf("%s: enough = true（n=%d）。90日に満たないはず", i.Label, i.N)
		}
	}
	if got.Note == "" {
		t.Error("Note が空。なぜ使えないかが分からない")
	}
}

func TestCorrelations_期間を指定できる(t *testing.T) {
	t.Parallel()
	d, _ := fixture(t, true)

	got, err := correlations(t.Context(), d, correlationsIn{Days: 30})
	if err != nil {
		t.Fatalf("correlations: %v", err)
	}
	if got.From == "" || got.To == "" {
		t.Errorf("期間が空: %+v", got)
	}
}
