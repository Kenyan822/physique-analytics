// Package plan は3年計画の設定（config.json）を読む。
//
// **実データは private/config.json にある**（ADR-0002）。このパッケージは
// パスを受け取って読むだけで、値をコードに持たない。公開できる例は
// リポジトリ直下の config.example.json。
//
// API サーバは使わない。手元で動かす MCP（ADR-0010）からだけ読む。
package plan

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// Plan は計画の設定。分析に必要な項目だけを持つ。
//
// config.json には他にも項目があるが、使っていないものは読まない。
// 読む項目が増えるほど、設定ファイルの形を変えにくくなる。
type Plan struct {
	HeightCm  float64              `json:"height_cm"`
	Phases    []Phase              `json:"phases"`
	Nutrition Nutrition            `json:"nutrition"`
	Volume    map[string]SetsRange `json:"volume_sets_per_muscle"`
}

// Phase は期間とその期間の目標ペース。
type Phase struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
	// GoalKgPerWeek は週あたりの体重変化の目標。負なら減量
	GoalKgPerWeek float64 `json:"goal_kg_per_week"`
}

// Nutrition は PFC の係数。
type Nutrition struct {
	Cut                Macros  `json:"cut"`
	DeepCut            Macros  `json:"deep_cut"`
	Bulk               Macros  `json:"bulk"`
	DeepCutBfThreshold float64 `json:"deep_cut_bf_threshold"`
	CarbMinG           float64 `json:"carb_min_g"`
}

// Macros は体重あたりのタンパク質・脂質。
type Macros struct {
	ProteinGPerKg float64 `json:"protein_g_per_kg"`
	FatGPerKg     float64 `json:"fat_g_per_kg"`
}

// SetsRange は部位別の MEV/MRV。
type SetsRange struct {
	Min int `json:"min"`
	Max int `json:"max"`
}

// defaultVolumeKey は部位別の設定が無いときに使うキー。
const defaultVolumeKey = "default"

// Load は設定ファイルを読む。
func Load(path string) (Plan, error) {
	b, err := os.ReadFile(path) //nolint:gosec // 手元の設定ファイルを読む。パスは利用者が指定する
	if err != nil {
		return Plan{}, fmt.Errorf("設定を読めない（%s）: %w", path, err)
	}

	var p Plan
	if err := json.Unmarshal(b, &p); err != nil {
		return Plan{}, fmt.Errorf("設定の形式が不正（%s）: %w", path, err)
	}

	return p, nil
}

// GoalAt は日付が属するフェーズの目標ペースを返す。
//
// **該当が無ければ ok=false を返す。** 0 を返すと維持期と区別が付かず、
// 停滞判定（維持期は横ばいが正常）が変わってしまう。
func (p Plan) GoalAt(date time.Time) (goalKgPerWeek float64, ok bool) {
	day := date.Format(time.DateOnly)

	for _, ph := range p.Phases {
		if ph.From <= day && day <= ph.To {
			return ph.GoalKgPerWeek, true
		}
	}

	return 0, false
}

// NutritionConfig は analytics が受け取る形に変換する。
func (p Plan) NutritionConfig() analytics.NutritionConfig {
	return analytics.NutritionConfig{
		Cut:                analytics.Macros(p.Nutrition.Cut),
		DeepCut:            analytics.Macros(p.Nutrition.DeepCut),
		Bulk:               analytics.Macros(p.Nutrition.Bulk),
		DeepCutBfThreshold: p.Nutrition.DeepCutBfThreshold,
		CarbMinG:           p.Nutrition.CarbMinG,
	}
}

// VolumeRange は部位の MEV/MRV を返す。設定が無ければ default に落ちる。
func (p Plan) VolumeRange(muscle string) (mev, mrv int) {
	if r, ok := p.Volume[muscle]; ok {
		return r.Min, r.Max
	}
	if r, ok := p.Volume[defaultVolumeKey]; ok {
		return r.Min, r.Max
	}

	return 0, 0
}
