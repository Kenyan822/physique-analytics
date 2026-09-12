package analytics_test

import (
	"strings"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

func TestWeeklyActions_問題が無ければ継続を返す(t *testing.T) {
	t.Parallel()

	got := analytics.WeeklyActions(nil, analytics.ActionContext{GoalKgPerWeek: -0.5})

	if len(got) != 1 {
		t.Fatalf("アクション = %d 件, want 1 件", len(got))
	}
	if got[0].Priority != analytics.PriorityInfo {
		t.Errorf("Priority = %v, want Info", got[0].Priority)
	}
	if !strings.Contains(got[0].Text, "継続") {
		t.Errorf("Text = %q, want 継続を含む", got[0].Text)
	}
}

func TestWeeklyActions_記録漏れを最優先にする(t *testing.T) {
	t.Parallel()

	// **記録が欠けているときは、他の判定より先に記録を直す。**
	// 前提が崩れているのに摂取やボリュームを動かすと、間違った方向に進む
	ds := []analytics.Detection{
		{Kind: analytics.StallWeightPlateau},
		{Kind: analytics.StallFatigue},
		{Kind: analytics.StallMissingRecords},
	}

	got := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5})

	if len(got) == 0 {
		t.Fatal("アクションが空")
	}
	if got[0].Kind != analytics.StallMissingRecords {
		t.Errorf("先頭 = %v, want missing_records", got[0].Kind)
	}
}

func TestWeeklyActions_優先順位(t *testing.T) {
	t.Parallel()

	// 回復 > 停滞 の順にする。疲労を抱えたまま摂取を削ると事故る
	ds := []analytics.Detection{
		{Kind: analytics.StallWeightPlateau},
		{Kind: analytics.StallHrvDrop},
	}

	got := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5})

	if got[0].Kind != analytics.StallHrvDrop {
		t.Errorf("先頭 = %v, want hrv_drop（回復を先に扱う）", got[0].Kind)
	}
}

func TestWeeklyActions_歩数の低下は停滞より先に出す(t *testing.T) {
	t.Parallel()

	// 「摂取を削る前に歩数を確認する」を順序で表す（docs/03-分析ロジック.md）
	ds := []analytics.Detection{
		{Kind: analytics.StallWeightPlateau},
		{Kind: analytics.StallStepsDrop},
	}

	got := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5})

	if got[0].Kind != analytics.StallStepsDrop {
		t.Errorf("先頭 = %v, want steps_drop", got[0].Kind)
	}
}

func TestWeeklyActions_停滞の文面はフェーズで変わる(t *testing.T) {
	t.Parallel()

	ds := []analytics.Detection{{Kind: analytics.StallWeightPlateau}}

	cut := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5})
	if !strings.Contains(cut[0].Text, "歩数") {
		t.Errorf("減量時の文面 = %q, want 歩数を含む", cut[0].Text)
	}

	bulk := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: 0.25})
	if !strings.Contains(bulk[0].Text, "増やす") {
		t.Errorf("増量時の文面 = %q, want 増やすを含む", bulk[0].Text)
	}
}

func TestWeeklyActions_摂取の下限に達していたら削らせない(t *testing.T) {
	t.Parallel()

	got := analytics.WeeklyActions(nil, analytics.ActionContext{
		GoalKgPerWeek:  -0.5,
		IntakeFloorHit: true,
	})

	if len(got) == 0 {
		t.Fatal("アクションが空")
	}
	// **ここから先は摂取を削らない。** 消費側で赤字を作る
	if !strings.Contains(got[0].Text, "消費") {
		t.Errorf("Text = %q, want 消費側で作る旨を含む", got[0].Text)
	}
}

func TestWeeklyActions_炭水化物の下限(t *testing.T) {
	t.Parallel()

	got := analytics.WeeklyActions(nil, analytics.ActionContext{
		GoalKgPerWeek:  -0.5,
		CarbBelowFloor: true,
		CarbTargetG:    180,
	})

	if len(got) == 0 {
		t.Fatal("アクションが空")
	}
	if !strings.Contains(got[0].Text, "180") {
		t.Errorf("Text = %q, want 目標値を含む", got[0].Text)
	}
}

func TestWeeklyActions_実測値を文面に入れる(t *testing.T) {
	t.Parallel()

	v, th := 4.2, 3.5
	ds := []analytics.Detection{{Kind: analytics.StallFatigue, Value: &v, Threshold: &th}}

	got := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5})

	// 数字が無いと「本当にそうなのか」を確かめられない
	if !strings.Contains(got[0].Text, "4.2") {
		t.Errorf("Text = %q, want 実測値 4.2 を含む", got[0].Text)
	}
}

func TestWeeklyActions_同じ種類は1つにまとめる(t *testing.T) {
	t.Parallel()

	ds := []analytics.Detection{
		{Kind: analytics.StallFatigue},
		{Kind: analytics.StallFatigue},
	}

	if got := analytics.WeeklyActions(ds, analytics.ActionContext{GoalKgPerWeek: -0.5}); len(got) != 1 {
		t.Errorf("アクション = %d 件, want 1 件", len(got))
	}
}

func TestWeeklyActions_文面が空にならない(t *testing.T) {
	t.Parallel()

	// 検知の種類を足したときに文面を書き忘れると、空行がレポートに出る
	kinds := []analytics.StallKind{
		analytics.StallWeightPlateau, analytics.StallIntakeUnstable,
		analytics.StallStrengthDrop, analytics.StallNoStrengthGain,
		analytics.StallFatigue, analytics.StallSleepShort,
		analytics.StallHrvDrop, analytics.StallRestingHrUp,
		analytics.StallDeepSleepShort, analytics.StallStepsDrop,
		analytics.StallMissingRecords,
	}

	for _, k := range kinds {
		t.Run(string(k), func(t *testing.T) {
			t.Parallel()
			got := analytics.WeeklyActions([]analytics.Detection{{Kind: k}},
				analytics.ActionContext{GoalKgPerWeek: -0.5})

			if len(got) != 1 {
				t.Fatalf("アクション = %d 件, want 1 件", len(got))
			}
			if strings.TrimSpace(got[0].Text) == "" {
				t.Error("文面が空")
			}
		})
	}
}

func TestWeeklyActions_大会のペースが速すぎる(t *testing.T) {
	t.Parallel()

	got := analytics.WeeklyActions(nil, analytics.ActionContext{
		GoalKgPerWeek:         -0.5,
		ContestPaceTooFast:    true,
		ContestWeeksLeft:      8,
		ContestPacePctPerWeek: 1.2,
	})

	if len(got) == 0 {
		t.Fatal("アクションが空")
	}
	if got[0].Kind != analytics.ActionContestPace {
		t.Errorf("先頭 = %v, want contest_pace", got[0].Kind)
	}
	// **数字を出す。** 「速すぎる」だけでは何週ぶん前倒せばいいか分からない
	if !strings.Contains(got[0].Text, "1.20") || !strings.Contains(got[0].Text, "8週") {
		t.Errorf("Text = %q, want 残り週数と必要ペースを含む", got[0].Text)
	}
}

func TestWeeklyActions_大会のペースは回復の次(t *testing.T) {
	t.Parallel()

	// 疲労を抱えたまま減量を速めるのは最悪の組み合わせなので、回復が先
	got := analytics.WeeklyActions(
		[]analytics.Detection{{Kind: analytics.StallFatigue}, {Kind: analytics.StallWeightPlateau}},
		analytics.ActionContext{GoalKgPerWeek: -0.5, ContestPaceTooFast: true, ContestWeeksLeft: 8},
	)

	kinds := make([]analytics.StallKind, 0, len(got))
	for _, a := range got {
		kinds = append(kinds, a.Kind)
	}
	if len(kinds) < 3 {
		t.Fatalf("アクション = %v", kinds)
	}
	if kinds[0] != analytics.StallFatigue {
		t.Errorf("先頭 = %v, want fatigue", kinds[0])
	}
	if kinds[1] != analytics.ActionContestPace {
		t.Errorf("2番目 = %v, want contest_pace", kinds[1])
	}
}
