package mcpserver

import (
	"context"
	"fmt"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// --- 食事の記録 ---
//
// **MCP で唯一の書き込み。** 写真からの PFC 推定を Claude アプリ側でやるための口で、
// サーバーに Anthropic の API キーを置かずに済ませるためにある（#297）。
// 分析系のツールと `query` は読み取りのままにする。

type logMealIn struct {
	Date     string   `json:"date,omitempty" jsonschema:"日付 YYYY-MM-DD（JST）。省略時は今日"`
	At       string   `json:"at,omitempty" jsonschema:"食べた時刻 HH:MM（JST）。省略可"`
	Slot     string   `json:"slot,omitempty" jsonschema:"朝食/昼食/夕食/間食。省略可"`
	Name     string   `json:"name,omitempty" jsonschema:"品名。「焼き鳥 5本」など"`
	Qty      string   `json:"qty,omitempty" jsonschema:"量。「中ジョッキ2杯」のような自由記述"`
	Kcal     *int     `json:"kcal,omitempty" jsonschema:"カロリー。省くと PFC から計算される"`
	ProteinG *float64 `json:"proteinG,omitempty" jsonschema:"タンパク質 g"`
	FatG     *float64 `json:"fatG,omitempty" jsonschema:"脂質 g"`
	CarbG    *float64 `json:"carbG,omitempty" jsonschema:"炭水化物 g"`
}

type logMealOut struct {
	ID   string `json:"id"`
	Date string `json:"date"`
	Name string `json:"name,omitempty"`
}

func registerMealTools(s *mcp.Server, d Deps) {
	if d.Meals == nil {
		return
	}

	mcp.AddTool(s, &mcp.Tool{
		Name: "log_meal",
		// **このサーバで唯一の書き込みツール。** 読み取り専用だと思って
		// 気軽に呼ばれないよう、明示する
		Annotations: &mcp.ToolAnnotations{
			ReadOnlyHint: false,
			// 行を足すだけ。既存の記録を消したり書き換えたりしない
			DestructiveHint: ptr(false),
			// 同じ内容で2回呼ぶと2行入る
			IdempotentHint: false,
		},
		Description: "食事を1件記録する。写真や口頭の説明から推定した PFC を入れる用途。" +
			"source は ai_estimated で固定され、手入力と区別できる。" +
			"**推定値をそのまま入れない。** 量が曖昧なときは qty に根拠を書き、" +
			"分からない栄養素は省く（省いた項目は null になり、未記録として扱われる）。",
	}, func(ctx context.Context, _ *mcp.CallToolRequest, in logMealIn) (*mcp.CallToolResult, logMealOut, error) {
		out, err := logMeal(ctx, d, in)

		return nil, out, err
	})
}

func logMeal(ctx context.Context, d Deps, in logMealIn) (logMealOut, error) {
	date, err := mealDate(in.Date, timeutil.Now())
	if err != nil {
		return logMealOut{}, err
	}

	input := repository.MealInput{
		Date: date,
		// **推定由来だと後から分かるようにする。** manual と混ざると
		// 推定の精度を評価できなくなる（要件 N-06）
		Source:   ptr(openapi.MealSourceAiEstimated),
		Kcal:     kcalOrDerived(in),
		ProteinG: f32ptr(in.ProteinG),
		FatG:     f32ptr(in.FatG),
		CarbG:    f32ptr(in.CarbG),
	}

	if in.At != "" {
		if _, err := time.Parse("15:04", in.At); err != nil {
			return logMealOut{}, fmt.Errorf("時刻は HH:MM で指定する: %q", in.At)
		}
		input.At = &in.At
	}
	if in.Slot != "" {
		slot := openapi.MealSlot(in.Slot)
		if !slot.Valid() {
			return logMealOut{}, fmt.Errorf("未知の slot: %q（朝食/昼食/夕食/間食）", in.Slot)
		}
		input.Slot = &slot
	}
	if in.Name != "" {
		input.Name = &in.Name
	}
	if in.Qty != "" {
		input.Qty = &in.Qty
	}

	m, err := d.Meals.Create(ctx, input)
	if err != nil {
		return logMealOut{}, err
	}

	out := logMealOut{ID: m.Id.String(), Date: m.Date.Format("2006-01-02")}
	if m.Name != nil {
		out.Name = *m.Name
	}

	return out, nil
}

// mealDate は空なら now の日（JST・ADR-0013）を返す。
//
// **Truncate(24h) を使わない。** 絶対時刻を切るのでロケーションを見ず、
// JST では9時に丸まる。朝9時前に記録すると前日の行になる。
//
// JST は「どの日か」を決めるためだけに使い、**返す Time は UTC の深夜**にする。
// date 列は日付そのもので、+09:00 のまま渡すと UTC に直されて前日になる。
func mealDate(s string, now time.Time) (openapi_types.Date, error) {
	if s == "" {
		return openapi_types.Date{Time: timeutil.StartOfDay(now)}, nil
	}

	t, err := time.Parse("2006-01-02", s)
	if err != nil {
		return openapi_types.Date{}, fmt.Errorf("日付は YYYY-MM-DD で指定する: %q", s)
	}

	return openapi_types.Date{Time: t}, nil
}

// kcalOrDerived は kcal が無ければ PFC から出す。
//
// **ツールの説明が「省くと PFC から計算される」と約束している。**
// ここで計算しないと LLM への説明が嘘になる。HTTP のハンドラは
// 同じことをしているが、MCP は repository を直接使うので通らない。
//
// **空欄は 0 とみなす。** 鶏むねの C のように「本当に 0」で空のまま
// 済ませる場面が普通にある（handler/meal.go と同じ規則）。
// ただし1つも入っていなければ計算しない。名前だけの記録に 0 kcal を付けない。
func kcalOrDerived(in logMealIn) *int {
	if in.Kcal != nil {
		return in.Kcal
	}
	if in.ProteinG == nil && in.FatG == nil && in.CarbG == nil {
		return nil
	}

	kcal := analytics.KcalFromMacros(zeroIfNil(in.ProteinG), zeroIfNil(in.FatG), zeroIfNil(in.CarbG))

	return &kcal
}

func zeroIfNil(v *float64) float64 {
	if v == nil {
		return 0
	}

	return *v
}

func ptr[T any](v T) *T { return &v }

// f32ptr は MCP が受ける float64 を、DB 側の float32 に落とす。
func f32ptr(v *float64) *float32 {
	if v == nil {
		return nil
	}
	f := float32(*v)

	return &f
}
