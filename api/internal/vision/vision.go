// Package vision は食事の写真から PFC を推定する（要件 N-06）。
//
// **食品マスタを持たない設計の弱点は「初めて食べるものの入力」**で、
// そこを Vision 対応の LLM で埋める（docs/01-要件定義.md §4.3）。
//
// 推定結果はそのまま保存しない。**必ず編集可能な形で提示し、
// 確認してから記録する。** 保存するときは source を ai_estimated にして、
// 後から精度を評価できるようにする。
package vision

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// defaultEndpoint は Anthropic Messages API。
const defaultEndpoint = "https://api.anthropic.com/v1/messages"

// apiVersion は Messages API のバージョンヘッダ。
const apiVersion = "2023-06-01"

// maxTokens は応答の上限。JSON 1つなので小さくてよい。
//
// **大きくすると事故ったときのコストが上がる。** 要件のコスト試算
// （年1,095回・出力200トークン程度）に合わせる。
const maxTokens = 512

// supportedMimeTypes は送れる画像形式。
//
// HEIC は API が受け付けないので、クライアント側で変換してから送る。
var supportedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/gif":  true,
}

// Config は推定に使う設定。
type Config struct {
	// APIKey が空なら推定を実行しない（課金が発生しない）
	APIKey string
	// Model は Vision 対応のモデル。要件のコスト試算は Haiku 相当
	Model string
	// Endpoint は差し替え用。空なら Anthropic の既定
	Endpoint string
}

// Request は推定の入力。
type Request struct {
	Image    []byte
	MimeType string
	// Note は任意のテキスト補足。**量を添えると精度が大きく上がる**
	// （写真だけでは食器のサイズが分からず量を推定しにくい）
	Note string
}

// Estimate は推定結果。**そのまま保存しない。**
type Estimate struct {
	Name     string   `json:"name"`
	Qty      *string  `json:"qty,omitempty"`
	Kcal     *int     `json:"kcal,omitempty"`
	ProteinG *float64 `json:"proteinG,omitempty"`
	FatG     *float64 `json:"fatG,omitempty"`
	CarbG    *float64 `json:"carbG,omitempty"`
	// Confidence は low / medium / high。量が分からないときは low になる
	Confidence string `json:"confidence,omitempty"`
	// Note は推定の根拠。ユーザーが直すときの手がかりになる
	Note string `json:"note,omitempty"`
}

// Client は Vision API のクライアント。
type Client struct {
	cfg  Config
	http *http.Client
}

// NewClient は Client を作る。
func NewClient(cfg Config, hc *http.Client) *Client {
	if hc == nil {
		hc = http.DefaultClient
	}

	return &Client{cfg: cfg, http: hc}
}

// Enabled は推定が使える状態かを返す。
func (c *Client) Enabled() bool { return c.cfg.APIKey != "" }

// prompt は LLM への指示。
//
// **JSON だけを返させる。** 説明文が混ざると壊れた推定値になるので、
// 読めなければエラーにする（握りつぶさない）。
const prompt = `この写真の食事の栄養素を推定してください。

出力は次の JSON のみ。説明文を付けないでください。

{"name":"料理名","qty":"量（例: 1食, 200g）","kcal":整数,"proteinG":数値,"fatG":数値,"carbG":数値,"confidence":"low|medium|high","note":"推定の根拠を1文で"}

- 量が判断できない場合は confidence を low にしてください
- 日本の一般的な食事量を前提にしてください
- 分からない項目は null にしてください`

// Estimate は写真とテキストから PFC を推定する。
func (c *Client) Estimate(ctx context.Context, req Request) (Estimate, error) {
	if c.cfg.APIKey == "" {
		return Estimate{}, errors.New(
			"写真からの推定が未設定。ANTHROPIC_API_KEY と VISION_MODEL を設定する")
	}
	if !supportedMimeTypes[req.MimeType] {
		return Estimate{}, fmt.Errorf("対応していない画像形式: %s（jpeg / png / webp / gif）", req.MimeType)
	}

	body, err := json.Marshal(c.messagesBody(req))
	if err != nil {
		return Estimate{}, fmt.Errorf("リクエストを組み立てられない: %w", err)
	}

	endpoint := c.cfg.Endpoint
	if endpoint == "" {
		endpoint = defaultEndpoint
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return Estimate{}, fmt.Errorf("リクエストを作れない: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	httpReq.Header.Set("x-api-key", c.cfg.APIKey)
	httpReq.Header.Set("anthropic-version", apiVersion)

	res, err := c.http.Do(httpReq)
	if err != nil {
		return Estimate{}, fmt.Errorf("推定 API に繋がらない: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return Estimate{}, fmt.Errorf("応答を読めない: %w", err)
	}

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// **握りつぶさない。** 課金が絡むので、失敗は見える形で残す
		return Estimate{}, fmt.Errorf("推定 API が %d を返した: %s",
			res.StatusCode, truncate(string(raw), 200))
	}

	return parseEstimate(raw)
}

func (c *Client) messagesBody(req Request) map[string]any {
	text := prompt
	if n := strings.TrimSpace(req.Note); n != "" {
		// 量を添えると精度が上がるので、補足は必ず渡す
		text += "\n\n補足: " + n
	}

	return map[string]any{
		"model":      c.cfg.Model,
		"max_tokens": maxTokens,
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{
					"type": "image",
					"source": map[string]string{
						"type":       "base64",
						"media_type": req.MimeType,
						"data":       base64.StdEncoding.EncodeToString(req.Image),
					},
				},
				{"type": "text", "text": text},
			},
		}},
	}
}

// parseEstimate は応答から JSON を取り出す。
func parseEstimate(raw []byte) (Estimate, error) {
	var res struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &res); err != nil {
		return Estimate{}, fmt.Errorf("応答の形式が不明: %w", err)
	}

	for _, c := range res.Content {
		if c.Type != "text" {
			continue
		}

		var e Estimate
		if err := json.Unmarshal([]byte(strings.TrimSpace(c.Text)), &e); err == nil && e.Name != "" {
			return e, nil
		}
	}

	// 説明文が返ってきた場合。**壊れた推定値を作らない**
	return Estimate{}, errors.New("推定結果を読み取れなかった。もう一度撮るか、手で入力する")
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "…"
}
