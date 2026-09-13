package weekly

import (
	"fmt"
	"strings"
	"time"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// 週次レポートの Markdown 出力（要件 A-13）。
//
// 節の構成は docs/03-分析ロジック.md の「週次レポートの構成」に合わせる。
// **測っていない項目の行は出さない。** 「—」が並ぶ表は読む気を失わせるし、
// 記録が足りないことが数字の欠落として伝わらない。

// Markdown は週次レポートを組み立てる。
//
// actions は analytics.WeeklyActions の結果。呼び出し側が停滞検知を
// 実行して渡す（この関数は整形だけを持つ）。
func Markdown(s Summary, actions []analytics.Action) string {
	var b strings.Builder

	fmt.Fprintf(&b, "# 週次レポート %s\n\n", s.AsOf.Format(time.DateOnly))
	if s.Phase != "" {
		fmt.Fprintf(&b, "**フェーズ**: %s / 目標ペース %+.2f kg/週\n\n", s.Phase, s.GoalKgPerWeek)
	}

	writeTrend(&b, s)
	writeContest(&b, s)
	writeDeviation(&b, s)
	writeIntake(&b, s)
	writeTaper(&b, s)
	writeActions(&b, actions)

	return b.String()
}

func writeTrend(b *strings.Builder, s Summary) {
	b.WriteString("## 1. 体重トレンド\n\n")

	rows := make([][2]string, 0, 5)
	if s.WeightKg7dAvg != nil {
		rows = append(rows, [2]string{"7日平均体重", fmt.Sprintf("%.2f kg", *s.WeightKg7dAvg)})
	}
	if s.WeightSlopeKgWeek != nil {
		rows = append(rows, [2]string{
			fmt.Sprintf("トレンド(%d日回帰)", analytics.TrendWindowDays),
			fmt.Sprintf("%+.2f kg/週", *s.WeightSlopeKgWeek),
		})
		rows = append(rows, [2]string{"目標との乖離",
			fmt.Sprintf("%+.2f kg/週", *s.WeightSlopeKgWeek-s.GoalKgPerWeek)})
	}
	if s.BodyfatPct7dAvg != nil {
		rows = append(rows, [2]string{"体脂肪率7日平均", fmt.Sprintf("%.1f %%", *s.BodyfatPct7dAvg)})
	}
	if c := s.Composition; c != nil {
		rows = append(rows, [2]string{"除脂肪体重(LBM)", fmt.Sprintf("%.1f kg", c.LbmKg)})
		rows = append(rows, [2]string{"**正規化FFMI**",
			fmt.Sprintf("**%.1f** (20-21=明らかに鍛えている / 22-23=ジムで目立つ)", c.Ffmi)})
	}

	writeTable(b, rows, "記録が足りないため出せない。")
}

func writeContest(b *strings.Builder, s Summary) {
	c := s.Contest
	if c == nil {
		return
	}

	fmt.Fprintf(b, "## 1.5 次の大会まで %.0f週 — %s %s\n\n",
		c.WeeksLeft, c.Contest.HeldOn.Format(time.DateOnly), c.Contest.Category)
	fmt.Fprintf(b, "目標ステージ体脂肪率 **%.1f%%**", float64(c.Contest.TargetBfPct))
	if c.Contest.Goal != nil && *c.Contest.Goal != "" {
		fmt.Fprintf(b, " / 位置づけ: %s", *c.Contest.Goal)
	}
	b.WriteString("\n\n")

	if t := c.Target; t != nil {
		writeTable(b, [][2]string{
			{"ステージ体重の目安", fmt.Sprintf("%.1f kg (LBM維持前提)", t.StageWeightKg)},
			{"必要な減量", fmt.Sprintf("%+.1f kg", t.NeedLossKg)},
			{"**必要ペース**", fmt.Sprintf("**%+.2f %%/週**", t.PacePctPerWeek)},
		}, "")
	}
}

func writeDeviation(b *strings.Builder, s Summary) {
	d := s.Deviation
	if d == nil {
		return
	}

	fmt.Fprintf(b, "## 2. 当月目標との進捗 (%s %s)\n\n", d.Month, d.Phase)
	writeTable(b, [][2]string{
		{"**LBM**", fmt.Sprintf("**%+.2f kg**", d.LbmKg)},
		{"体脂肪率", fmt.Sprintf("%+.2f pt", d.BodyfatPct)},
		{"体重", fmt.Sprintf("%+.2f kg", d.WeightKg)},
		{"正規化FFMI", fmt.Sprintf("%+.2f", d.Ffmi)},
	}, "")
	b.WriteString("追う順序は **LBM → 体脂肪率 → 体重**。体重は最後に決まる数字で、目標ではない。\n\n")
}

func writeIntake(b *strings.Builder, s Summary) {
	b.WriteString("## 3. 推定TDEE と 来週の推奨摂取\n\n")

	if s.TDEEKcal == nil || s.Targets == nil {
		note := s.Note
		if note == "" {
			note = "データ不足で推定できない。"
		}
		b.WriteString(note + "\n\n")

		return
	}

	t := s.Targets
	rows := [][2]string{
		{"**推定TDEE**", fmt.Sprintf("**%.0f kcal**", *s.TDEEKcal)},
		{"**来週の推奨摂取**", fmt.Sprintf("**%.0f kcal/日**", t.KcalTarget)},
		{"タンパク質", fmt.Sprintf("%.0f g", t.ProteinG)},
		{"脂質", fmt.Sprintf("%.0f g", t.FatG)},
		{"炭水化物", fmt.Sprintf("%.0f g", t.CarbG)},
	}
	writeTable(b, rows, "")

	if t.IntakeFloorHit {
		b.WriteString("> 摂取が下限に達している。これ以上削らず、消費側で赤字を作る。\n\n")
	}
	if t.CarbBelowFloor {
		b.WriteString("> 炭水化物が下限を割っている。歩数を上げて赤字を作る。\n\n")
	}
}

func writeTaper(b *strings.Builder, s Summary) {
	t := s.Taper
	if t == nil {
		return
	}

	fmt.Fprintf(b, "## 7. リカバリ / 周囲長 (直近計測 %s)\n\n", t.Date.Format(time.DateOnly))

	rows := make([][2]string, 0, 2)
	if t.NavyBodyfatPct != nil {
		rows = append(rows, [2]string{"海軍式 推定体脂肪率",
			fmt.Sprintf("%.1f %% (体組成計とは独立した推定)", *t.NavyBodyfatPct)})
	}
	if sw := t.ShoulderWaist; sw != nil {
		verdict := "目標 1.60 以上"
		if sw.VTaper {
			verdict = "V字が成立するレンジ"
		}
		rows = append(rows, [2]string{"**肩/ウエスト比**", fmt.Sprintf("**%.3f** (%s)", sw.Ratio, verdict)})
	}
	writeTable(b, rows, "")
}

func writeActions(b *strings.Builder, actions []analytics.Action) {
	b.WriteString("## 8. 今週のアクション\n\n")

	if len(actions) == 0 {
		b.WriteString("計画どおり。変更なし。現在の摂取とボリュームを継続する。\n")

		return
	}
	for i, a := range actions {
		fmt.Fprintf(b, "%d. %s\n", i+1, a.Text)
	}
	b.WriteString("\n")
}

// writeTable は2列の表を書く。行が無ければ empty を出す（空の表を出さない）。
func writeTable(b *strings.Builder, rows [][2]string, empty string) {
	if len(rows) == 0 {
		if empty != "" {
			b.WriteString(empty + "\n\n")
		}

		return
	}

	b.WriteString("| 指標 | 値 |\n|---|---|\n")
	for _, r := range rows {
		fmt.Fprintf(b, "| %s | %s |\n", r[0], r[1])
	}
	b.WriteString("\n")
}
