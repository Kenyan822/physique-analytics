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
	"net/url"
	"strings"
)

// defaultEndpoint は Vertex AI のベース URL。
//
// Google AI Studio（generativelanguage.googleapis.com）ではない。
// **AI Studio の無料枠は請求を紐付けていないプロジェクトでしか効かず**、
// このプロジェクトは Cloud Run のために請求が要る（#304）。
// Vertex なら鍵を持たずにサービスアカウントで呼べる。
const defaultEndpoint = "https://aiplatform.googleapis.com/v1"

// location はモデルを呼ぶ場所。global は空きのあるリージョンに回してくれる。
const location = "global"

// thinkingLevel は思考の深さ。
//
// **Gemini 3 系は思考を切れない。** gemini-3.8-flash の既定は medium で、
// minimal を指定すると 400 になる。写真1枚から PFC を出すだけなので low で足りる。
const thinkingLevel = "low"

// supportedMimeTypes は送れる画像形式。
//
// **Gemini が受け付ける形式に合わせてある。** HEIC が通るので iPhone の写真は
// 変換せずに送れる。GIF は受け付けない。
var supportedMimeTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
	"image/heic": true,
	"image/heif": true,
}

// Config は推定に使う設定。
type Config struct {
	// ProjectID が空なら推定を実行しない（課金が発生しない）。
	//
	// **鍵は持たない。** 認証は NewClient に渡す *http.Client の責務で、
	// 本番では Cloud Run のサービスアカウント（ADC）が署名する。
	// 認証情報が取れないときは ProjectID を立てずに「使えない」状態にする
	ProjectID string
	// Model は Vision 対応のモデル
	Model string
	// Endpoint は差し替え用のベース URL。空なら Vertex の既定
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
func (c *Client) Enabled() bool { return c.cfg.ProjectID != "" }

// estimateSchema は structured output のスキーマ。
//
// **形は prompt ではなくここで縛る。** 説明文が混ざった応答を防ぐ。
// 分からない項目は null を許す（0 で埋めると偽の推定値になる）。
var estimateSchema = map[string]any{
	"type": "object",
	"properties": map[string]any{
		"name":     map[string]any{"type": "string", "description": "料理名"},
		"qty":      map[string]any{"type": []string{"string", "null"}, "description": "量。例: 1食、200g"},
		"kcal":     map[string]any{"type": []string{"integer", "null"}},
		"proteinG": map[string]any{"type": []string{"number", "null"}},
		"fatG":     map[string]any{"type": []string{"number", "null"}},
		"carbG":    map[string]any{"type": []string{"number", "null"}},
		"confidence": map[string]any{
			"type": "string", "enum": []string{"low", "medium", "high"},
			"description": "量が判断できなければ low",
		},
		"note": map[string]any{"type": "string", "description": "推定の根拠を1文で"},
	},
	"required": []string{"name", "confidence", "note"},
}

// prompt は LLM への指示。
//
// **出力の形は estimateSchema が縛る。** ここには形から読み取れない前提だけ書く。
const prompt = `この写真の食事の栄養素を推定してください。

- 量が判断できない場合は confidence を low にしてください
- 日本の一般的な食事量を前提にしてください
- 分からない項目は null にしてください`

// Estimate は写真とテキストから PFC を推定する。
func (c *Client) Estimate(ctx context.Context, req Request) (Estimate, error) {
	if c.cfg.ProjectID == "" {
		return Estimate{}, errors.New(
			"写真からの推定が未設定。VISION_PROJECT_ID を設定する")
	}
	if !supportedMimeTypes[req.MimeType] {
		return Estimate{}, fmt.Errorf(
			"対応していない画像形式: %s（jpeg / png / webp / heic / heif）", req.MimeType)
	}

	body, err := json.Marshal(c.generateContentBody(req))
	if err != nil {
		return Estimate{}, fmt.Errorf("リクエストを組み立てられない: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint(), bytes.NewReader(body))
	if err != nil {
		return Estimate{}, fmt.Errorf("リクエストを作れない: %w", err)
	}
	httpReq.Header.Set("content-type", "application/json")
	// **Authorization はここで付けない。** c.http がサービスアカウントで署名する

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
		// **握りつぶさない。** 課金とクォータの問題もここに出る
		return Estimate{}, fmt.Errorf("推定 API が %d を返した: %s",
			res.StatusCode, truncate(string(raw), 200))
	}

	return parseEstimate(raw)
}

// endpoint は呼び先を組み立てる。
// **Vertex はプロジェクトもモデルも URL で指定する**（Anthropic はボディだった）。
func (c *Client) endpoint() string {
	base := c.cfg.Endpoint
	if base == "" {
		base = defaultEndpoint
	}

	return fmt.Sprintf("%s/projects/%s/locations/%s/publishers/google/models/%s:generateContent",
		strings.TrimSuffix(base, "/"), url.PathEscape(c.cfg.ProjectID),
		location, url.PathEscape(c.cfg.Model))
}

func (c *Client) generateContentBody(req Request) map[string]any {
	text := prompt
	if n := strings.TrimSpace(req.Note); n != "" {
		// 量を添えると精度が上がるので、補足は必ず渡す
		text += "\n\n補足: " + n
	}

	return map[string]any{
		"contents": []map[string]any{{
			// **Vertex は role を省略できない。** 無いと 400 になる
			// （AI Studio は既定で user を補っていた）
			"role": "user",
			"parts": []map[string]any{
				{"inline_data": map[string]string{
					"mime_type": req.MimeType,
					"data":      base64.StdEncoding.EncodeToString(req.Image),
				}},
				// 画像とテキストを混ぜるときは、テキストを後ろに置く（Gemini の推奨）
				{"text": text},
			},
		}},
		"generationConfig": map[string]any{
			"thinkingConfig": map[string]any{"thinkingLevel": thinkingLevel},
			"responseFormat": map[string]any{
				"text": map[string]any{
					// **enum なので MIME 文字列ではない。**
					// "application/json" を渡すと 400 になる（実際に踏んだ）
					"mimeType": "APPLICATION_JSON",
					"schema":   estimateSchema,
				},
			},
		},
		// **maxOutputTokens は設定しない。** thinking の分も数える hard cutoff なので、
		// 小さくすると本文が空のまま 200 が返る。絞るなら thinkingLevel を下げる
	}
}

// generateContentResponse は応答のうち、推定に使う部分だけ。
type generateContentResponse struct {
	Candidates []struct {
		Content struct {
			Parts []struct {
				Text string `json:"text"`
			} `json:"parts"`
		} `json:"content"`
		FinishReason string `json:"finishReason"`
	} `json:"candidates"`
}

// parseEstimate は応答から JSON を取り出す。
func parseEstimate(raw []byte) (Estimate, error) {
	var res generateContentResponse
	if err := json.Unmarshal(raw, &res); err != nil {
		return Estimate{}, fmt.Errorf("応答の形式が不明: %w", err)
	}

	for _, cand := range res.Candidates {
		for _, p := range cand.Content.Parts {
			var e Estimate
			if err := json.Unmarshal([]byte(strings.TrimSpace(p.Text)), &e); err == nil && e.Name != "" {
				return e, nil
			}
		}
	}

	// 安全フィルタや打ち切りで本文が返らないことがある。**壊れた推定値を作らない**
	return Estimate{}, fmt.Errorf("推定結果を読み取れなかった%s。もう一度撮るか、手で入力する",
		res.finishNote())
}

// finishNote は打ち切りの理由を添える。
// MAX_TOKENS / SAFETY のどれで止まったか分からないと直せない。
func (r generateContentResponse) finishNote() string {
	for _, cand := range r.Candidates {
		if cand.FinishReason != "" && cand.FinishReason != "STOP" {
			return "（" + cand.FinishReason + "）"
		}
	}

	return ""
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}

	return s[:n] + "…"
}
