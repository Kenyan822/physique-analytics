package handler

import (
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/vision"
)

const (
	// maxImageBytes は受け取る画像の上限。
	// **大きい画像は精度を上げない**（API 側で縮小される）。
	// 送信時間とトークンだけが増えるので、入口で止める。
	maxImageBytes = 8 << 20

	// maxNoteLength はテキスト補足の上限。
	maxNoteLength = 500
)

// EstimateMeal は写真から PFC を推定する（要件 N-06）。
//
// **推定結果は保存しない。** 編集可能な下書きとして返し、
// 確認してから POST /v1/meals で記録する。
func (s *Server) EstimateMeal(ctx context.Context, req openapi.EstimateMealRequestObject) (openapi.EstimateMealResponseObject, error) {
	if s.estimator == nil || !s.estimator.Enabled() {
		return openapi.EstimateMeal503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: openapi.ServiceUnavailableApplicationProblemPlusJSONResponse(
				problem(503, "写真からの推定が未設定",
					"ANTHROPIC_API_KEY と VISION_MODEL を設定すると使える。手で入力することもできる")),
		}, nil
	}
	if req.Body == nil {
		return estimateFailed("image", "リクエストボディが無い"), nil
	}

	image, mimeType, note, err := readEstimateForm(req.Body)
	if err != nil {
		return estimateFailed("image", err.Error()), nil
	}

	est, err := s.estimator.Estimate(ctx, vision.Request{
		Image: image, MimeType: mimeType, Note: note,
	})
	if err != nil {
		// **推定の失敗は 503 で返す。** リクエストが悪いわけではないので、
		// 利用者は「手で入力する」に切り替えればよい
		return openapi.EstimateMeal503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: openapi.ServiceUnavailableApplicationProblemPlusJSONResponse(
				problem(503, "推定できなかった", err.Error())),
		}, nil
	}

	out := openapi.MealEstimate{
		Name: est.Name,
		Qty:  est.Qty,
		Kcal: est.Kcal,
		// 記録するときも ai_estimated を保つ（docs/02-データモデル.md）
		Source: openapi.MealSourceAiEstimated,
	}
	out.ProteinG = float32Ptr(est.ProteinG)
	out.FatG = float32Ptr(est.FatG)
	out.CarbG = float32Ptr(est.CarbG)
	if est.Confidence != "" {
		c := est.Confidence
		out.Confidence = &c
	}
	if est.Note != "" {
		n := est.Note
		out.Note = &n
	}

	return openapi.EstimateMeal200JSONResponse(out), nil
}

// readEstimateForm は multipart から画像と補足を取り出す。
func readEstimateForm(r *multipart.Reader) (image []byte, mimeType, note string, err error) {
	for {
		part, e := r.NextPart()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return nil, "", "", fmt.Errorf("フォームを読めない: %w", e)
		}

		switch part.FormName() {
		case "image":
			mimeType = part.Header.Get("Content-Type")
			// 上限+1 まで読んで、超えていたら弾く
			image, e = io.ReadAll(io.LimitReader(part, maxImageBytes+1))
			if e != nil {
				return nil, "", "", fmt.Errorf("画像を読めない: %w", e)
			}
			if len(image) > maxImageBytes {
				return nil, "", "", fmt.Errorf("画像が大きすぎる（上限 %dMB）", maxImageBytes>>20)
			}
		case "note":
			b, e := io.ReadAll(io.LimitReader(part, maxNoteLength*4))
			if e != nil {
				return nil, "", "", fmt.Errorf("補足を読めない: %w", e)
			}
			note = strings.TrimSpace(string(b))
		}
		if e := part.Close(); e != nil {
			return nil, "", "", fmt.Errorf("フォームを閉じられない: %w", e)
		}
	}

	if len(image) == 0 {
		return nil, "", "", fmt.Errorf("画像が無い")
	}
	if len([]rune(note)) > maxNoteLength {
		return nil, "", "", fmt.Errorf("補足は%d文字以下にする", maxNoteLength)
	}

	return image, mimeType, note, nil
}

func float32Ptr(v *float64) *float32 {
	if v == nil {
		return nil
	}
	f := float32(*v)

	return &f
}

func estimateFailed(field, message string) openapi.EstimateMeal422ApplicationProblemPlusJSONResponse {
	return openapi.EstimateMeal422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
