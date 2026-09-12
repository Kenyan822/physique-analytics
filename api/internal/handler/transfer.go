package handler

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"strings"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/csvio"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// maxUploadBytes は取り込む CSV の上限。
// 3年分のセット記録でも数 MB に収まる。無制限にするとメモリを使い切れる。
const maxUploadBytes = 16 << 20 // 16MiB

// ExportCsv は CSV を書き出す。
//
// `reference/analysis/` の入力形式であり、長期バックアップの正でもある
// （ADR-0002 / ADR-0011）。**スキーマを変えると両方が壊れる。**
func (s *Server) ExportCsv(ctx context.Context, req openapi.ExportCsvRequestObject) (openapi.ExportCsvResponseObject, error) {
	var buf bytes.Buffer

	switch req.Params.Resource {
	case openapi.ExportCsvParamsResourceDaily:
		rows, err := s.transfer.ExportDaily(ctx, req.Params.From, req.Params.To)
		if err != nil {
			return nil, err
		}
		if err := csvio.WriteDaily(&buf, rows); err != nil {
			return nil, err
		}

	case openapi.ExportCsvParamsResourceWorkouts:
		rows, err := s.transfer.ExportWorkouts(ctx, req.Params.From, req.Params.To)
		if err != nil {
			return nil, err
		}
		if err := csvio.WriteWorkouts(&buf, rows); err != nil {
			return nil, err
		}

	case openapi.ExportCsvParamsResourceMeasures:
		rows, err := s.transfer.ExportMeasures(ctx, req.Params.From, req.Params.To)
		if err != nil {
			return nil, err
		}
		if err := csvio.WriteMeasures(&buf, rows); err != nil {
			return nil, err
		}

	default:
		return exportBadRequest("resource が不正",
			fmt.Sprintf("%q は未知。daily / workouts / measures のいずれか", req.Params.Resource)), nil
	}

	return openapi.ExportCsv200TextcsvResponse{
		Body:          bytes.NewReader(buf.Bytes()),
		ContentLength: int64(buf.Len()),
	}, nil
}

// ImportCsv は CSV を取り込む（要件 I-01）。
//
// **1行のミスで全部止めない。** 読めた行は取り込み、落とした行は
// 行番号つきで返す。手で書いた過去データには必ず穴がある。
func (s *Server) ImportCsv(ctx context.Context, req openapi.ImportCsvRequestObject) (openapi.ImportCsvResponseObject, error) {
	if req.Body == nil {
		return importValidationFailed("リクエストボディが無い"), nil
	}

	resource, onDup, file, err := readImportForm(req.Body)
	if err != nil {
		return importValidationFailed(err.Error()), nil
	}

	var res repository.ImportResult

	switch resource {
	case "daily":
		rows, rowErrs, err := csvio.ParseDaily(file)
		if err != nil {
			return importValidationFailed(err.Error()), nil
		}
		res, err = s.transfer.ImportDaily(ctx, rows, onDup)
		if err != nil {
			return nil, err
		}
		res.Errors = append(rowErrs, res.Errors...)

	case "workouts":
		rows, rowErrs, err := csvio.ParseWorkouts(file)
		if err != nil {
			return importValidationFailed(err.Error()), nil
		}
		res, err = s.transfer.ImportWorkouts(ctx, rows, onDup)
		if err != nil {
			return nil, err
		}
		res.Errors = append(rowErrs, res.Errors...)

	case "measures":
		rows, rowErrs, err := csvio.ParseMeasures(file)
		if err != nil {
			return importValidationFailed(err.Error()), nil
		}
		res, err = s.transfer.ImportMeasures(ctx, rows, onDup)
		if err != nil {
			return nil, err
		}
		res.Errors = append(rowErrs, res.Errors...)

	default:
		return importValidationFailed(
			fmt.Sprintf("resource %q は未知。daily / workouts / measures のいずれか", resource)), nil
	}

	out := openapi.ImportCsv200JSONResponse{
		Imported: res.Imported,
		Skipped:  res.Skipped,
		Errors:   make([]openapi.ImportError, 0, len(res.Errors)),
	}
	for _, e := range res.Errors {
		out.Errors = append(out.Errors, openapi.ImportError{Line: e.Line, Message: e.Message})
	}

	return out, nil
}

// readImportForm は multipart から resource / onDuplicate / file を取り出す。
func readImportForm(mr *multipart.Reader) (resource string, onDup repository.OnDuplicate, file io.Reader, err error) {
	// 既定は skip。取り込みで既存を壊さない
	onDup = repository.OnDuplicateSkip

	var body []byte
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", "", nil, fmt.Errorf("multipart を読めない: %w", err)
		}

		switch part.FormName() {
		case "resource":
			v, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				return "", "", nil, fmt.Errorf("resource を読めない: %w", err)
			}
			resource = strings.TrimSpace(string(v))

		case "onDuplicate":
			v, err := io.ReadAll(io.LimitReader(part, 64))
			if err != nil {
				return "", "", nil, fmt.Errorf("onDuplicate を読めない: %w", err)
			}
			if strings.TrimSpace(string(v)) == string(repository.OnDuplicateOverwrite) {
				onDup = repository.OnDuplicateOverwrite
			}

		case "file":
			// LimitReader で上限を掛ける。無制限だとメモリを使い切れる
			body, err = io.ReadAll(io.LimitReader(part, maxUploadBytes+1))
			if err != nil {
				return "", "", nil, fmt.Errorf("file を読めない: %w", err)
			}
			if len(body) > maxUploadBytes {
				return "", "", nil, fmt.Errorf("file が大きすぎる（%d バイトまで）", maxUploadBytes)
			}
		}
		_ = part.Close()
	}

	if resource == "" {
		return "", "", nil, fmt.Errorf("resource が無い")
	}
	if body == nil {
		return "", "", nil, fmt.Errorf("file が無い")
	}

	return resource, onDup, bytes.NewReader(body), nil
}

func exportBadRequest(title, detail string) openapi.ExportCsv400ApplicationProblemPlusJSONResponse {
	return openapi.ExportCsv400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}

func importValidationFailed(message string) openapi.ImportCsv422ApplicationProblemPlusJSONResponse {
	p := problem(422, "取り込めない", message)
	p.Errors = &[]struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}{{Field: "file", Message: message}}

	return openapi.ImportCsv422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(p),
	}
}
