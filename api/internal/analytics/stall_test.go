package analytics_test

import (
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
)

// has は kind の検知が含まれているかを返す。
func has(ds []analytics.Detection, kind analytics.StallKind) bool {
	for _, d := range ds {
		if d.Kind == kind {
			return true
		}
	}

	return false
}

// cutting は減量中で何も問題が無い入力。各テストはここから1項目だけ崩す。
func cutting() analytics.StallInput {
	return analytics.StallInput{
		GoalKgPerWeek:     -0.5,
		WeightSlopeKgWeek: ptrF(-0.48),
		KcalCVPct:         ptrF(6),
		E1RMSlopeKgWeek:   ptrF(-0.05),
		Fatigue7dAvg:      ptrF(2.0),
		SleepH7dAvg:       ptrF(7.2),
		Hrv7dAvg:          ptrF(66),
		Hrv30dAvg:         ptrF(68),
		RestingHr7dAvg:    ptrF(53),
		RestingHr30dAvg:   ptrF(52),
		DeepSleepMin7dAvg: ptrF(75),
	}
}

func TestDetectStalls_問題が無ければ何も返さない(t *testing.T) {
	t.Parallel()

	if got := analytics.DetectStalls(cutting()); len(got) != 0 {
		t.Errorf("検知 = %+v, want 0件", got)
	}
}

func TestDetectStalls_減量停滞(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*analytics.StallInput)
		wantKind  analytics.StallKind
		wantFound bool
	}{
		{
			// 傾きが 0.1kg/週 未満で、摂取が安定している（CV < 10%）
			name:      "横ばい かつ 摂取が安定なら停滞",
			mutate:    func(in *analytics.StallInput) { in.WeightSlopeKgWeek = ptrF(-0.04) },
			wantKind:  analytics.StallWeightPlateau,
			wantFound: true,
		},
		{
			// **摂取がばらついているときは停滞と断定しない。**
			// 代謝適応より記録漏れの方がずっと多い
			name: "横ばいでも摂取がばらついていれば記録を疑う",
			mutate: func(in *analytics.StallInput) {
				in.WeightSlopeKgWeek = ptrF(-0.04)
				in.KcalCVPct = ptrF(18)
			},
			wantKind:  analytics.StallIntakeUnstable,
			wantFound: true,
		},
		{
			name: "摂取のばらつきが不明でも記録を疑う",
			mutate: func(in *analytics.StallInput) {
				in.WeightSlopeKgWeek = ptrF(-0.04)
				in.KcalCVPct = nil
			},
			wantKind:  analytics.StallIntakeUnstable,
			wantFound: true,
		},
		{
			// 維持期は横ばいが正しい状態なので停滞ではない
			name: "目標が維持なら横ばいでも停滞にしない",
			mutate: func(in *analytics.StallInput) {
				in.GoalKgPerWeek = 0
				in.WeightSlopeKgWeek = ptrF(-0.04)
			},
			wantKind:  analytics.StallWeightPlateau,
			wantFound: false,
		},
		{
			name:      "体重の傾きが不明なら判定しない",
			mutate:    func(in *analytics.StallInput) { in.WeightSlopeKgWeek = nil },
			wantKind:  analytics.StallWeightPlateau,
			wantFound: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := cutting()
			tt.mutate(&in)

			if got := has(analytics.DetectStalls(in), tt.wantKind); got != tt.wantFound {
				t.Errorf("%s の検知 = %v, want %v", tt.wantKind, got, tt.wantFound)
			}
		})
	}
}

func TestDetectStalls_各条件(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name      string
		mutate    func(*analytics.StallInput)
		wantKind  analytics.StallKind
		wantFound bool
	}{
		{"e1RM が -0.3kg/週 以下で筋力低下", func(in *analytics.StallInput) {
			in.E1RMSlopeKgWeek = ptrF(-0.35)
		}, analytics.StallStrengthDrop, true},
		{"e1RM が -0.3kg/週 ちょうどでも検知する（仕様は「以下」）", func(in *analytics.StallInput) {
			in.E1RMSlopeKgWeek = ptrF(-0.3)
		}, analytics.StallStrengthDrop, true},
		{"e1RM が -0.29kg/週 なら検知しない", func(in *analytics.StallInput) {
			in.E1RMSlopeKgWeek = ptrF(-0.29)
		}, analytics.StallStrengthDrop, false},
		{"疲労度の7日平均が 3.5 超で回復不足", func(in *analytics.StallInput) {
			in.Fatigue7dAvg = ptrF(3.6)
		}, analytics.StallFatigue, true},
		{"睡眠の7日平均が 6.5h 未満で回復不足", func(in *analytics.StallInput) {
			in.SleepH7dAvg = ptrF(6.4)
		}, analytics.StallSleepShort, true},
		{"HRV が30日基準から -15% で回復不足", func(in *analytics.StallInput) {
			in.Hrv7dAvg = ptrF(57) // 68 * 0.85 = 57.8 を下回る
		}, analytics.StallHrvDrop, true},
		{"HRV の低下が -15% 未満なら検知しない", func(in *analytics.StallInput) {
			in.Hrv7dAvg = ptrF(59)
		}, analytics.StallHrvDrop, false},
		{"HRV の基準が無ければ判定しない", func(in *analytics.StallInput) {
			in.Hrv7dAvg, in.Hrv30dAvg = ptrF(30), nil
		}, analytics.StallHrvDrop, false},
		{"安静時心拍が基準 +5bpm 超で回復不足", func(in *analytics.StallInput) {
			in.RestingHr7dAvg = ptrF(58) // 基準 52 に対して +6
		}, analytics.StallRestingHrUp, true},
		{"安静時心拍が +5bpm ちょうどなら検知しない", func(in *analytics.StallInput) {
			in.RestingHr7dAvg = ptrF(57)
		}, analytics.StallRestingHrUp, false},
		{"深睡眠が60分未満で睡眠の質を疑う", func(in *analytics.StallInput) {
			in.DeepSleepMin7dAvg = ptrF(52)
		}, analytics.StallDeepSleepShort, true},
		{"摂取の欠損が週2日以上で記録漏れ", func(in *analytics.StallInput) {
			in.MissingKcalDays = 2
		}, analytics.StallMissingRecords, true},
		{"体重の欠損が週2日以上でも記録漏れ", func(in *analytics.StallInput) {
			in.MissingWeightDays = 3
		}, analytics.StallMissingRecords, true},
		{"欠損が1日なら検知しない", func(in *analytics.StallInput) {
			in.MissingKcalDays = 1
		}, analytics.StallMissingRecords, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			in := cutting()
			tt.mutate(&in)

			if got := has(analytics.DetectStalls(in), tt.wantKind); got != tt.wantFound {
				t.Errorf("%s の検知 = %v, want %v", tt.wantKind, got, tt.wantFound)
			}
		})
	}
}

func TestDetectStalls_増量期は閾値が変わる(t *testing.T) {
	t.Parallel()

	// 増量期に期待される傾きは +0.3〜+1.0kg/週。**横ばいで既に異常**なので、
	// 減量期の -0.3 を当てると見逃す
	bulk := func(slope float64) analytics.StallInput {
		in := cutting()
		in.GoalKgPerWeek = 0.25
		in.WeightSlopeKgWeek = ptrF(0.24)
		in.E1RMSlopeKgWeek = ptrF(slope)

		return in
	}

	if !has(analytics.DetectStalls(bulk(-0.05)), analytics.StallNoStrengthGain) {
		t.Error("増量中の横ばいを検知していない")
	}
	if has(analytics.DetectStalls(bulk(0.5)), analytics.StallNoStrengthGain) {
		t.Error("伸びているのに検知している")
	}
	// 減量期の種類は出さない。文面（A-08）が「減量ペースを緩める」になってしまう
	if has(analytics.DetectStalls(bulk(-0.5)), analytics.StallStrengthDrop) {
		t.Error("増量中に減量期用の検知が出ている")
	}
}

func TestDetectStalls_歩数の低下(t *testing.T) {
	t.Parallel()

	// **減量停滞の主犯はほぼこれ。** 摂取を削る前に歩数を戻す
	in := cutting()
	in.Steps7dAvg, in.StepsPrev7dAvg = ptrF(6000), ptrF(8000)

	if !has(analytics.DetectStalls(in), analytics.StallStepsDrop) {
		t.Error("歩数の低下を検知していない")
	}

	// 増量中は歩数が落ちても問題にしない
	in.GoalKgPerWeek = 0.25
	if has(analytics.DetectStalls(in), analytics.StallStepsDrop) {
		t.Error("増量中に歩数の低下を検知している")
	}
}

func TestDetectStalls_検知した値を持って返す(t *testing.T) {
	t.Parallel()

	in := cutting()
	in.Fatigue7dAvg = ptrF(4.2)

	for _, d := range analytics.DetectStalls(in) {
		if d.Kind != analytics.StallFatigue {
			continue
		}
		// メッセージの組み立て（A-08）で使うので、判定に使った値を返す
		if d.Value == nil || *d.Value != 4.2 {
			t.Errorf("Value = %v, want 4.2", d.Value)
		}
		if d.Threshold == nil || *d.Threshold != 3.5 {
			t.Errorf("Threshold = %v, want 3.5", d.Threshold)
		}

		return
	}
	t.Fatal("疲労の検知が無い")
}
