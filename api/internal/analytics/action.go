package analytics

import (
	"fmt"
	"slices"
)

// 「今週のアクション」（要件 A-08）。
//
// **これが最終成果物**（docs/01-要件定義.md）。数字の羅列ではなく
// 「来週何を変えるか」が出ることをゴールにする。他の分析はすべてその根拠。

// ActionPriority はアクションの優先度。並べ替えと表示の強弱に使う。
type ActionPriority int

const (
	// PriorityBlocker は他の判断の前提が壊れている状態。先に直す
	PriorityBlocker ActionPriority = iota
	// PriorityRecovery は回復に関わる。放置すると怪我と筋力低下に直結する
	PriorityRecovery
	// PriorityAdjust は計画の微調整
	PriorityAdjust
	// PriorityInfo は変更不要の報告
	PriorityInfo
)

// ActionContext は検知だけでは決まらない文脈。
//
// 摂取・炭水化物の下限（要件 A-03）は「検知」ではなく計算結果なので、
// DetectStalls ではなくこちらから渡す。
type ActionContext struct {
	GoalKgPerWeek float64
	// IntakeFloorHit は推奨摂取が下限に達したか（RecommendedIntake の FloorHit）
	IntakeFloorHit bool
	// CarbBelowFloor / CarbTargetG は炭水化物の目標が下限を割ったか
	CarbBelowFloor bool
	CarbTargetG    float64

	// ContestPaceTooFast は大会までに必要なペースが安全域を超えたか（要件 A-10）
	ContestPaceTooFast    bool
	ContestWeeksLeft      float64
	ContestPacePctPerWeek float64
}

// Action は「来週やること」1件。
type Action struct {
	Kind     StallKind
	Priority ActionPriority
	Text     string
}

// actionFloorIntake / actionFloorCarb は検知（StallKind）ではない項目に
// 便宜上つける種類。表示の並べ替えで他と同じ扱いにするために持つ。
const (
	ActionIntakeFloor StallKind = "intake_floor"
	ActionCarbFloor   StallKind = "carb_floor"
	ActionOnPlan      StallKind = "on_plan"
	ActionContestPace StallKind = "contest_pace"
)

// priorityOf は種類ごとの優先度。
//
// **順序そのものが判断**。記録 → 回復 → 消費側 → 摂取 の順にしてある。
// 記録が欠けたまま摂取を動かすと、間違った方向に進む。疲労を抱えたまま
// 摂取を削ると事故る。歩数を確認する前に摂取を削るのが一番多い間違い。
func priorityOf(kind StallKind) ActionPriority {
	switch kind {
	case StallMissingRecords, StallIntakeUnstable:
		return PriorityBlocker
	case StallHrvDrop, StallRestingHrUp, StallDeepSleepShort, StallFatigue, StallSleepShort,
		StallStrengthDrop, StallNoStrengthGain:
		return PriorityRecovery
	case ActionContestPace:
		// 大会のペースは計画そのものを変える話なので、回復の次に置く
		return PriorityRecovery
	case StallStepsDrop, ActionIntakeFloor, ActionCarbFloor, StallWeightPlateau:
		return PriorityAdjust
	default:
		return PriorityAdjust
	}
}

// order は同じ優先度の中での並び順。小さいほど先。
var order = map[StallKind]int{
	StallMissingRecords: 0,
	StallIntakeUnstable: 1,

	StallHrvDrop:        0,
	StallRestingHrUp:    1,
	StallDeepSleepShort: 2,
	StallFatigue:        3,
	StallSleepShort:     4,
	StallStrengthDrop:   5,
	StallNoStrengthGain: 6,
	ActionContestPace:   7,

	// 摂取を削る前に消費側を確認する
	StallStepsDrop:     0,
	ActionIntakeFloor:  1,
	ActionCarbFloor:    2,
	StallWeightPlateau: 3,
}

// WeeklyActions は検知と文脈から、優先順位を付けたアクションを返す。
//
// 何も無ければ「継続」を1件返す。空を返さないのは、レポートに
// 「変更なし」が明示されないと、判定が動いていないのか問題が無いのかが
// 区別できないため。
func WeeklyActions(ds []Detection, ctx ActionContext) []Action {
	byKind := make(map[StallKind]Detection, len(ds))
	for _, d := range ds {
		// 同じ種類が複数来たら最初の1つだけ使う。主要種目ごとに
		// 筋力低下が出ると同じ文面が並ぶ
		if _, ok := byKind[d.Kind]; !ok {
			byKind[d.Kind] = d
		}
	}

	out := make([]Action, 0, len(byKind)+2)
	for kind, d := range byKind {
		out = append(out, Action{Kind: kind, Priority: priorityOf(kind), Text: textFor(kind, d, ctx)})
	}

	if ctx.ContestPaceTooFast {
		out = append(out, Action{
			Kind:     ActionContestPace,
			Priority: priorityOf(ActionContestPace),
			Text: fmt.Sprintf("大会までのペースが速すぎる。残り%.0f週で %.2f%%/週 が必要"+
				"（安全域は %.1f%%/週 まで）。このまま追うと LBM を失う。"+
				"減量開始を前倒しするか、ステージ体脂肪率の目標を緩める。",
				ctx.ContestWeeksLeft, ctx.ContestPacePctPerWeek, SafePacePctPerWeek),
		})
	}
	if ctx.IntakeFloorHit {
		out = append(out, Action{
			Kind:     ActionIntakeFloor,
			Priority: priorityOf(ActionIntakeFloor),
			Text: "摂取が下限（体重×24kcal）に達している。ここから先は摂取を削らず、" +
				"消費側（歩数・有酸素）で赤字を作る。削り続けると LBM 損失と代謝適応が進む。",
		})
	}
	if ctx.CarbBelowFloor {
		out = append(out, Action{
			Kind:     ActionCarbFloor,
			Priority: priorityOf(ActionCarbFloor),
			Text: fmt.Sprintf("炭水化物の目標が %.0fg まで下がっている。トレーニングのボリュームを"+
				"支えられない。摂取をこれ以上削らず、歩数を上げて消費側で赤字を作る。", ctx.CarbTargetG),
		})
	}

	if len(out) == 0 {
		return []Action{{
			Kind:     ActionOnPlan,
			Priority: PriorityInfo,
			Text:     "計画どおり。変更なし。現在の摂取とボリュームを継続する。",
		}}
	}

	slices.SortStableFunc(out, func(a, b Action) int {
		if a.Priority != b.Priority {
			return int(a.Priority) - int(b.Priority)
		}

		return order[a.Kind] - order[b.Kind]
	})

	return out
}

// textFor は検知1件の文面を組み立てる。
//
// **実測値を入れる。** 「疲労が溜まっている」だけでは確かめようがなく、
// 翌週に効いたかどうかも判定できない。
func textFor(kind StallKind, d Detection, ctx ActionContext) string {
	cutting := ctx.GoalKgPerWeek < 0

	switch kind {
	case StallWeightPlateau:
		if cutting {
			return "体重が横ばい" + valueSuffix(d, "%+.2f kg/週") +
				"で摂取も安定している。歩数 +2000/日 を先に試し、それでも動かなければ摂取 -150kcal。"
		}

		return "体重が横ばい" + valueSuffix(d, "%+.2f kg/週") + "。摂取を +200kcal 増やす。"

	case StallIntakeUnstable:
		return "体重は横ばいだが摂取のばらつきが大きい" + valueSuffix(d, "CV %.1f%%") +
			"。摂取を動かす前に3日間の厳密計量で記録精度を確かめる。"

	case StallStrengthDrop:
		return "主要種目の e1RM が落ちている" + valueSuffix(d, "%+.2f kg/週") +
			"。ボリュームを削るか、減量ペースを1段緩める。"

	case StallNoStrengthGain:
		return "増量中なのに e1RM が伸びていない" + valueSuffix(d, "%+.2f kg/週") +
			"。ボリューム・回復・摂取のどれが足りていないかを切り分ける。"

	case StallFatigue:
		return "疲労度が高い状態が続いている" + valueSuffix(d, "7日平均 %.1f/5") +
			"。回復週を前倒しする。"

	case StallSleepShort:
		return "睡眠が不足している" + valueSuffix(d, "7日平均 %.1fh") +
			"。筋力低下と停滞の主要因になる。減量ペースより睡眠を優先する。"

	case StallHrvDrop:
		return "HRV が30日基準から落ちている" + hrvSuffix(d) +
			"。自律神経の疲労は体感より早く出る。回復週を前倒しするか、減量中ならペースを1段落とす。"

	case StallRestingHrUp:
		return "安静時心拍が基準より上がっている" + valueSuffix(d, "%+.0f bpm") +
			"。HRV と合わせて判断し、両方が悪化していれば回復を優先する。"

	case StallDeepSleepShort:
		return "深睡眠が足りていない" + valueSuffix(d, "7日平均 %.0f分 / 目安60分") +
			"。睡眠時間が足りていても質が低い。減量ペースより睡眠を優先する。"

	case StallStepsDrop:
		return "歩数が前週より落ちている" + valueSuffix(d, "%+.0f 歩/日") +
			"。減量停滞の主犯はたいていこれ。摂取を削る前に歩数を戻す。"

	case StallMissingRecords:
		return "直近7日の記録に欠損がある" + valueSuffix(d, "%.0f日") +
			"。分析の前提が崩れるので、摂取やボリュームを動かす判断より記録の復旧が先。"

	default:
		return "確認が必要な項目がある。"
	}
}

// valueSuffix は実測値があれば「（…）」で添える。無ければ空文字。
func valueSuffix(d Detection, format string) string {
	if d.Value == nil {
		return ""
	}

	return "（" + fmt.Sprintf(format, *d.Value) + "）"
}

// hrvSuffix は HRV だけ比率を百分率に直して出す。
func hrvSuffix(d Detection) string {
	if d.Value == nil {
		return ""
	}

	return fmt.Sprintf("（%.0f%%）", (*d.Value-1)*100)
}
