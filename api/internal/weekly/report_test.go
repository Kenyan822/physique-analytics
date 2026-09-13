package weekly_test

import (
	"strings"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/analytics"
	"github.com/Kenyan822/physique-analytics/api/internal/weekly"
)

func TestMarkdown_節の順序(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:         &stubPlan{plan: plan(), blocks: blocks()},
		Series:       &stubSeries{points: series(21, 74, 20, 2100)},
		Measurements: &stubMeasurements{m: measurement(39, 120, 75)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	md := weekly.Markdown(got, nil)

	// docs/03-分析ロジック.md の「週次レポートの構成」と同じ順序
	want := []string{
		"## 1. 体重トレンド",
		"## 3. 推定TDEE と 来週の推奨摂取",
		"## 7. リカバリ / 周囲長",
		"## 8. 今週のアクション",
	}
	at := 0
	for _, w := range want {
		i := strings.Index(md[at:], w)
		if i < 0 {
			t.Fatalf("%q が %d 文字目以降に無い\n---\n%s", w, at, md)
		}
		at += i
	}
}

func TestMarkdown_アクションが無ければ継続と書く(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	// 検知が無い状態を直接渡す。Markdown 側の振る舞いを見るテスト
	md := weekly.Markdown(got, nil)
	if !strings.Contains(md, "継続") {
		t.Errorf("継続の記述が無い\n---\n%s", md)
	}
}

func TestMarkdown_アクションを番号つきで並べる(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	actions := analytics.WeeklyActions(
		[]analytics.Detection{{Kind: analytics.StallFatigue}}, got.ActionContext())

	md := weekly.Markdown(got, actions)
	if !strings.Contains(md, "1. 疲労度が高い") {
		t.Errorf("番号つきで出ていない\n---\n%s", md)
	}
}

func TestMarkdown_TDEEを出せないときは理由を書く(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(3, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	md := weekly.Markdown(got, nil)
	// **数字が出ないだけだと、壊れているのか記録が足りないのか分からない**
	if !strings.Contains(md, "TDEE を推定できない") {
		t.Errorf("理由が無い\n---\n%s", md)
	}
}

func TestMarkdown_測っていない項目の行を出さない(t *testing.T) {
	t.Parallel()

	got, err := weekly.Build(t.Context(), weekly.Deps{
		Plan:   &stubPlan{plan: plan()},
		Series: &stubSeries{points: series(21, 74, 20, 2100)},
	}, asof)
	if err != nil {
		t.Fatalf("Build: %v", err)
	}

	md := weekly.Markdown(got, nil)
	// 周囲長を渡していないので、その行は出ない
	if strings.Contains(md, "肩/ウエスト比") {
		t.Errorf("測っていない項目が出ている\n---\n%s", md)
	}
}
