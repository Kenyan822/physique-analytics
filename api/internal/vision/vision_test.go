package vision_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// geminiBody は Gemini の応答を組み立てる。
// 本文は candidates[].content.parts[].text に入る（Anthropic の content[] とは違う）。
func geminiBody(t *testing.T, text string) string {
	t.Helper()

	b, err := json.Marshal(map[string]any{
		"candidates": []map[string]any{{
			"content": map[string]any{
				"parts": []map[string]any{{"text": text}},
			},
		}},
	})
	if err != nil {
		t.Fatalf("応答を組み立てられない: %v", err)
	}

	return string(b)
}

// noNetwork は「入口で止まるはず」のテストに渡す。
//
// **弾かれなかったときに本物の API へ出て行かないようにするため。**
// 実際に、移行前のテストが許可リストの変更で素通りして 401 を貰っている。
type noNetwork struct{ t *testing.T }

func (n noNetwork) RoundTrip(r *http.Request) (*http.Response, error) {
	n.t.Helper()
	n.t.Errorf("API を呼んでいる: %s", r.URL)

	return nil, errors.New("外に出てはいけない")
}

func TestEstimate_応答をPFCにする(t *testing.T) {
	t.Parallel()

	// structured output を指定しているので JSON だけが返る
	body := geminiBody(t, `{"name":"鶏むねと白米","qty":"1食","kcal":620,"proteinG":48,"fatG":6,"carbG":88,"confidence":"medium","note":"食器のサイズから量を推定"}`)
	tr := &fakeTransport{status: 200, body: body}

	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "gemini-3.8-flash"},
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

func TestEstimate_リクエストの形(t *testing.T) {
	t.Parallel()

	tr := &fakeTransport{status: 200, body: geminiBody(t, `{"name":"x"}`)}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "gemini-3.8-flash"},
		&http.Client{Transport: tr})

	img := []byte{0x89, 0x50, 0x4e, 0x47}
	if _, err := c.Estimate(context.Background(), vision.Request{
		Image: img, MimeType: "image/jpeg", Note: "鶏むね200g、白米150g",
	}); err != nil {
		t.Fatalf("Estimate: %v", err)
	}

	// **このパッケージは鍵を知らない。** 認証は渡された http.Client の責務
	// （Cloud Run のサービスアカウントで署名する）
	if got := tr.gotReq.Header.Get("x-goog-api-key"); got != "" {
		t.Errorf("x-goog-api-key = %q, want 空。鍵は持たない", got)
	}

	// Vertex はプロジェクトとモデルを URL のパスで指定する
	want := "/projects/p-1/locations/global/publishers/google/models/gemini-3.8-flash:generateContent"
	if u := tr.gotReq.URL.String(); !strings.Contains(u, want) {
		t.Errorf("URL = %s, want %s を含む", u, want)
	}

	// **Vertex は role を省略できない。** 無いと 400 になる（AI Studio は既定で補う）
	if !strings.Contains(tr.gotRaw, `"role":"user"`) {
		t.Errorf("role が無い: %s", truncate(tr.gotRaw))
	}

	// 画像は inline_data で送る
	if !strings.Contains(tr.gotRaw, "inline_data") {
		t.Errorf("inline_data が無い: %s", truncate(tr.gotRaw))
	}
	if enc := base64.StdEncoding.EncodeToString(img); !strings.Contains(tr.gotRaw, enc) {
		t.Errorf("画像が base64 で送られていない: %s", truncate(tr.gotRaw))
	}

	// **量を添えると精度が大きく上がる**（要件 N-06）。確実に送る
	if !strings.Contains(tr.gotRaw, "鶏むね200g") {
		t.Errorf("テキスト補足が送られていない: %s", truncate(tr.gotRaw))
	}

	// structured output を指定して、説明文が混ざるのを防ぐ。
	// **mimeType は enum。** "application/json" だと 400 になる
	if !strings.Contains(tr.gotRaw, "responseFormat") || !strings.Contains(tr.gotRaw, `"APPLICATION_JSON"`) {
		t.Errorf("structured output を指定していない: %s", truncate(tr.gotRaw))
	}
}

func TestEstimate_プロジェクトが無ければエラー(t *testing.T) {
	t.Parallel()

	c := vision.NewClient(vision.Config{}, &http.Client{Transport: noNetwork{t}})

	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	// **直し方を書く。** 「使えない」だけでは次の行動が決まらない
	if !strings.Contains(err.Error(), "VISION_PROJECT_ID") {
		t.Errorf("err = %v, want 設定方法を含む", err)
	}
}

func TestEstimate_失敗を握りつぶさない(t *testing.T) {
	t.Parallel()

	tr := &fakeTransport{status: 429, body: `{"error":{"message":"rate limited"}}`}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: tr})

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

	// structured output を指定していても、安全フィルタなどで説明文が返ることがある。
	// **壊れた推定値を作らない**
	tr := &fakeTransport{status: 200, body: geminiBody(t, "この写真は鶏むねです。")}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: tr})

	if _, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"}); err == nil {
		t.Error("エラーにならない")
	}
}

func TestEstimate_候補が空(t *testing.T) {
	t.Parallel()

	// 安全フィルタで止まると candidates が空で 200 が返る
	tr := &fakeTransport{status: 200, body: `{"candidates":[]}`}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: tr})

	if _, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"}); err == nil {
		t.Error("エラーにならない")
	}
}

func TestEstimate_打ち切られたら理由を返す(t *testing.T) {
	t.Parallel()

	// **maxOutputTokens は thinking の分も数える。** 絞りすぎると本文が空で 200 が返る。
	// 何が起きたか分からないと直せないので、理由をエラーに載せる
	tr := &fakeTransport{status: 200, body: `{"candidates":[{"finishReason":"MAX_TOKENS","content":{"parts":[]}}]}`}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: tr})

	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/jpeg"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "MAX_TOKENS") {
		t.Errorf("err = %v, want 打ち切りの理由を含む", err)
	}
}

func TestEstimate_対応していない形式(t *testing.T) {
	t.Parallel()

	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: noNetwork{t}})

	// Gemini は HEIC を受け付けるが GIF は受け付けない
	_, err := c.Estimate(context.Background(), vision.Request{Image: []byte{1}, MimeType: "image/gif"})
	if err == nil {
		t.Fatal("エラーにならない")
	}
	if !strings.Contains(err.Error(), "image/gif") {
		t.Errorf("err = %v, want 形式名を含む", err)
	}
}

func TestEstimate_HEICを受け付ける(t *testing.T) {
	t.Parallel()

	// iPhone の写真はそのまま HEIC。**変換せずに送れる**
	tr := &fakeTransport{status: 200, body: geminiBody(t, `{"name":"x"}`)}
	c := vision.NewClient(vision.Config{ProjectID: "p-1", Model: "m"}, &http.Client{Transport: tr})

	if _, err := c.Estimate(context.Background(), vision.Request{
		Image: []byte{1}, MimeType: "image/heic",
	}); err != nil {
		t.Errorf("Estimate: %v", err)
	}
}

func truncate(s string) string {
	if len(s) <= 300 {
		return s
	}

	return s[:300] + "…"
}
