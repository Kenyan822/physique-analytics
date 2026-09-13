package vision_test

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/Kenyan822/physique-analytics/api/internal/vision"
)

// fakeTransport は実 API に繋がずに応答を返す。
// **開発中に課金を発生させないため**、テストは必ずこれを通す。
type fakeTransport struct {
	status int
	body   string
	gotReq *http.Request
	gotRaw string
}

func (f *fakeTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	f.gotReq = r
	if r.Body != nil {
		var sb strings.Builder
		buf := make([]byte, 4096)
		for {
			n, err := r.Body.Read(buf)
			sb.Write(buf[:n])
			if err != nil {
				break
			}
		}
		f.gotRaw = sb.String()
	}

	return &http.Response{
		StatusCode: f.status,
		Body:       io.NopCloser(strings.NewReader(f.body)),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}, nil
}

func TestEstimate_応答をPFCにする(t *testing.T) {
	t.Parallel()

	// LLM は JSON だけを返すよう指示してある
	body := `{"content":[{"type":"text","text":"{\"name\":\"鶏むねと白米\",\"qty\":\"1食\",\"kcal\":620,\"proteinG\":48,\"fatG\":6,\"carbG\":88,\"confidence\":\"medium\",\"note\":\"食器のサイズから量を推定\"}"}]}`
	tr := &fakeTransport{status: 200, body: body}

	c := vision.NewClient(vision.Config{APIKey: "test-key", Model: "claude-haiku-4-5-20251001"},
		&http.Client{Transport: tr})

	got, err := c.Estimate(context.Background(), vision.Request{
		Image: []byte{0x89, 0x50}, MimeType: "image/jpeg", Note: "鶏むね200g",
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	if got.Name != "鶏むねと白米" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Kcal == nil || *got.Kcal != 620 {
		t.Errorf("Kcal = %v, want 620", got.Kcal)
	}
	if got.Confidence != "medium" {
		t.Errorf("Confidence = %q", got.Confidence)
	}
}

func TestEstimate_テキスト補足を送る(t *testing.T) {
	t.Parallel()

	tr := &fakeTransport{status: 200, body: `{"content":[{"type":"text","text":"{\"name\":\"x\"}"}]}`}
	c := vision.NewClient(vision.Config{APIKey: "k", Model: "m"}, &http.Client{Transport: tr})

	_, err := c.Estimate(context.Background(), vision.Request{
		Image: []byte{1}, MimeType: "image/jpeg", Note: "鶏むね200g、白米150g",
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	// **量を添えると精度が大きく上がる**（要件 N-06）。確実に送る
	if !strings.Contains(tr.gotRaw, "鶏むね200g") {
		t.Errorf("テキスト補足が送られていない: %s", tr.gotRaw[:min(300, len(tr.gotRaw))])
	}
	// 画像は base64 で送る
	if !strings.Contains(tr.gotRaw, "base64") {
		t.Error("画像が base64 で送られていない")
	}
}

func TestEstimate_APIキーが無ければエラー(t *testing.T) {
	t.Parallel()

	c := vision.NewClient(vision.Config{}, http.DefaultClient)

	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	// **直し方を書く。** 「使えない」だけでは次の行動が決まらない
	if !strings.Contains(err.Error(), "ANTHROPIC_API_KEY") {
		t.Errorf("err = %v, want 設定方法を含む", err)
	}
}

func TestEstimate_失敗を握りつぶさない(t *testing.T) {
	t.Parallel()

	tr := &fakeTransport{status: 429, body: `{"error":{"message":"rate limited"}}`}
	c := vision.NewClient(vision.Config{APIKey: "k", Model: "m"}, &http.Client{Transport: tr})

	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "429") {
		t.Errorf("err = %v, want ステータスを含む", err)
	}
}

func TestEstimate_JSONでない応答(t *testing.T) {
	t.Parallel()

	// LLM が説明文を返すことがある。**壊れた推定値を作らない**
	tr := &fakeTransport{status: 200, body: `{"content":[{"type":"text","text":"この写真は鶏むねです。"}]}`}
	c := vision.NewClient(vision.Config{APIKey: "k", Model: "m"}, &http.Client{Transport: tr})

	if _, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"}); err == nil {
		t.Error("エラーにならない")
	}
}

func TestEstimate_対応していない形式(t *testing.T) {
	t.Parallel()

	c := vision.NewClient(vision.Config{APIKey: "k", Model: "m"}, http.DefaultClient)

	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/heic"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "image/heic") {
		t.Errorf("err = %v, want 形式名を含む", err)
	}
}
