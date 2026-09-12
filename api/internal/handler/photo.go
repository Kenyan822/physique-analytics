package handler

import (
	"context"
	"fmt"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/blobstore"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

const (
	// viewURLExpiry は閲覧用の署名付きURLの寿命。
	// **短くする。** 転送中に漏れても効力が切れる（ADR-0008）
	viewURLExpiry = 15 * time.Minute

	// uploadURLExpiry はアップロード用。撮影から送信までの時間を見込む
	uploadURLExpiry = 10 * time.Minute

	// maxPhotoBytes は受け付ける上限。iPhone の写真は3〜5MB
	maxPhotoBytes = 20 << 20
)

// ListPhotos は写真を署名付きURL付きで返す（要件 B-04 / B-07）。
func (s *Server) ListPhotos(ctx context.Context, req openapi.ListPhotosRequestObject) (openapi.ListPhotosResponseObject, error) {
	if resp := s.photosUnavailable(); resp != nil {
		return openapi.ListPhotos503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	rows, err := s.photos.List(ctx, req.Params.From, req.Params.To)
	if err != nil {
		return nil, err
	}

	return openapi.ListPhotos200JSONResponse{Items: s.withURLs(rows)}, nil
}

// GetPhotoGuide は撮影ガイド用に前回写真を返す（要件 B-05）。
func (s *Server) GetPhotoGuide(ctx context.Context, req openapi.GetPhotoGuideRequestObject) (openapi.GetPhotoGuideResponseObject, error) {
	if resp := s.photosUnavailable(); resp != nil {
		return openapi.GetPhotoGuide503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}

	before := openapi_types.Date{Time: timeutil.Now().Truncate(24 * time.Hour)}
	if req.Params.Before != nil {
		before = *req.Params.Before
	}

	rows, err := s.photos.Guide(ctx, before)
	if err != nil {
		return nil, err
	}

	return openapi.GetPhotoGuide200JSONResponse{Items: s.withURLs(rows)}, nil
}

// CreatePhotoUpload はアップロード用の署名付きURLを発行する（要件 B-04）。
//
// **画像は API を経由させない。** 4MB の画像を Cloud Run に通すと、
// メモリもリクエスト時間も無駄になる。
func (s *Server) CreatePhotoUpload(ctx context.Context, req openapi.CreatePhotoUploadRequestObject) (openapi.CreatePhotoUploadResponseObject, error) {
	if resp := s.photosUnavailable(); resp != nil {
		return openapi.CreatePhotoUpload503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: *resp,
		}, nil
	}
	if req.Body == nil {
		return photoFailed("body", "リクエストボディが無い"), nil
	}
	if msg := validatePhotoInput(*req.Body); msg != "" {
		return photoFailed("byteSize", msg), nil
	}

	date := req.Body.Date.Format("2006-01-02")
	key := blobstore.PhotoKey(date, string(req.Body.Pose), string(req.Body.MimeType))

	row, err := s.photos.Create(ctx, repository.PhotoInput{
		Date:       req.Body.Date,
		Pose:       req.Body.Pose,
		StorageKey: key,
		MimeType:   string(req.Body.MimeType),
		ByteSize:   int64(req.Body.ByteSize),
		Note:       req.Body.Note,
	})
	if repository.IsConflict(err) {
		return photoFailed("pose",
			fmt.Sprintf("%s の %s は既にある。撮り直すなら先に消す", date, req.Body.Pose)), nil
	}
	if err != nil {
		return nil, err
	}

	uploadURL, err := s.blobs.PresignPut(key, uploadURLExpiry, timeutil.Now())
	if err != nil {
		return nil, err
	}

	photo := row.Photo
	if url, err := s.blobs.PresignGet(key, viewURLExpiry, timeutil.Now()); err == nil {
		photo.Url = &url
	}

	return openapi.CreatePhotoUpload201JSONResponse{Photo: photo, UploadUrl: uploadURL}, nil
}

// DeletePhoto は写真を論理削除する。
func (s *Server) DeletePhoto(ctx context.Context, req openapi.DeletePhotoRequestObject) (openapi.DeletePhotoResponseObject, error) {
	if s.photos == nil {
		return openapi.DeletePhoto404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "写真が見つからない", "")),
		}, nil
	}

	err := s.photos.SoftDelete(ctx, req.PhotoId)
	if repository.IsNotFound(err) {
		return openapi.DeletePhoto404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "写真が見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeletePhoto204Response{}, nil
}

// withURLs は短期の署名付きURLを添える。**公開URLは作らない**（ADR-0008）。
func (s *Server) withURLs(rows []repository.PhotoRow) []openapi.BodyPhoto {
	out := make([]openapi.BodyPhoto, 0, len(rows))
	now := timeutil.Now()

	for _, r := range rows {
		p := r.Photo
		// URL の発行に失敗しても一覧は返す。メタデータだけでも
		// 「いつ撮ったか」は分かる
		if url, err := s.blobs.PresignGet(r.StorageKey, viewURLExpiry, now); err == nil {
			p.Url = &url
		}
		out = append(out, p)
	}

	return out
}

func (s *Server) photosUnavailable() *openapi.ServiceUnavailableApplicationProblemPlusJSONResponse {
	if s.photos != nil && s.blobs != nil && s.blobs.Enabled() {
		return nil
	}

	p := openapi.ServiceUnavailableApplicationProblemPlusJSONResponse(
		problem(503, "写真の保存先が未設定",
			"R2_ACCOUNT_ID / R2_BUCKET / R2_ACCESS_KEY_ID / R2_SECRET_ACCESS_KEY を設定する"))

	return &p
}

func validatePhotoInput(in openapi.PhotoUploadInput) string {
	if in.ByteSize <= 0 || in.ByteSize > maxPhotoBytes {
		return fmt.Sprintf("サイズは 1 〜 %dMB にする", maxPhotoBytes>>20)
	}
	if in.Note != nil && len([]rune(*in.Note)) > 500 {
		return "メモは500文字以下にする"
	}

	return ""
}

func photoFailed(field, message string) openapi.CreatePhotoUpload422ApplicationProblemPlusJSONResponse {
	return openapi.CreatePhotoUpload422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}
