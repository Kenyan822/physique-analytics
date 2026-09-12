package handler

import (
	"context"
	"fmt"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// maxTemplateItems はテンプレート1つあたりの項目数の上限。
// 要件 T-05（1日最大4種目）に対して十分な余裕を取る。
const maxTemplateItems = 20

// ListTemplates はテンプレートの一覧を返す。
func (s *Server) ListTemplates(ctx context.Context, _ openapi.ListTemplatesRequestObject) (openapi.ListTemplatesResponseObject, error) {
	items, err := s.templates.List(ctx)
	if err != nil {
		return nil, err
	}
	// nil のままだと JSON が items: null になり、required: [items] を満たさない
	if items == nil {
		items = []openapi.Template{}
	}

	return openapi.ListTemplates200JSONResponse{Items: items}, nil
}

// GetTemplate はテンプレートを1件返す。
func (s *Server) GetTemplate(ctx context.Context, req openapi.GetTemplateRequestObject) (openapi.GetTemplateResponseObject, error) {
	tpl, err := s.templates.Get(ctx, req.TemplateId)
	if repository.IsNotFound(err) {
		return openapi.GetTemplate404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "テンプレートが見つからない", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.GetTemplate200JSONResponse(tpl), nil
}

// CreateTemplate はテンプレートを作る。
func (s *Server) CreateTemplate(ctx context.Context, req openapi.CreateTemplateRequestObject) (openapi.CreateTemplateResponseObject, error) {
	if req.Body == nil {
		return templateBadRequest("リクエストボディが無い", ""), nil
	}
	if msg := validateTemplateInput(*req.Body); msg != "" {
		return templateBadRequest("入力が不正", msg), nil
	}

	tpl, err := s.templates.Create(ctx, toTemplateInput(*req.Body))
	if repository.IsConflict(err) {
		return templateBadRequest("項目の順序が重複している", err.Error()), nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.CreateTemplate201JSONResponse(tpl), nil
}

// UpdateTemplate はテンプレートを更新する。項目は全入れ替え。
func (s *Server) UpdateTemplate(ctx context.Context, req openapi.UpdateTemplateRequestObject) (openapi.UpdateTemplateResponseObject, error) {
	if req.Body == nil {
		return updateTemplateBadRequest("リクエストボディが無い", ""), nil
	}
	if msg := validateTemplateInput(*req.Body); msg != "" {
		return updateTemplateBadRequest("入力が不正", msg), nil
	}

	tpl, err := s.templates.Update(ctx, req.TemplateId, toTemplateInput(*req.Body))
	switch {
	case repository.IsNotFound(err):
		return openapi.UpdateTemplate404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "テンプレートが見つからない", "")),
		}, nil
	case repository.IsConflict(err):
		return openapi.UpdateTemplate409ApplicationProblemPlusJSONResponse{
			ConflictApplicationProblemPlusJSONResponse: openapi.ConflictApplicationProblemPlusJSONResponse(
				problem(409, "項目の順序が重複している", err.Error())),
		}, nil
	case err != nil:
		return nil, err
	}

	return openapi.UpdateTemplate200JSONResponse(tpl), nil
}

// DeleteTemplate はテンプレートを論理削除する。
func (s *Server) DeleteTemplate(ctx context.Context, req openapi.DeleteTemplateRequestObject) (openapi.DeleteTemplateResponseObject, error) {
	err := s.templates.SoftDelete(ctx, req.TemplateId)
	if repository.IsNotFound(err) {
		// openapi.yaml は deleteTemplate に 404 を定義していない。
		// 「消えている」状態は達成されているので 204 で返す（冪等）
		return openapi.DeleteTemplate204Response{}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteTemplate204Response{}, nil
}

func validateTemplateInput(in openapi.TemplateInput) string {
	switch {
	case in.Name == "":
		return "name が空。1文字以上必要"
	case len([]rune(in.Name)) > 100:
		return "name が長すぎる（100文字まで）"
	case len(in.Items) == 0:
		return "items が空。1つ以上の種目が必要"
	case len(in.Items) > maxTemplateItems:
		return fmt.Sprintf("items が多すぎる（%d まで）", maxTemplateItems)
	}

	seen := map[int]bool{}
	for i, it := range in.Items {
		switch {
		case it.Order < 1:
			return fmt.Sprintf("items[%d].order が範囲外: %d（1 以上）", i, it.Order)
		case seen[it.Order]:
			return fmt.Sprintf("items[%d].order が重複: %d", i, it.Order)
		case it.TargetSets < 1 || it.TargetSets > 20:
			return fmt.Sprintf("items[%d].targetSets が範囲外: %d（1〜20）", i, it.TargetSets)
		case it.TargetRir != nil && (*it.TargetRir < 0 || *it.TargetRir > maxRIR):
			return fmt.Sprintf("items[%d].targetRir が範囲外: %d（0〜%d）", i, *it.TargetRir, maxRIR)
		}
		seen[it.Order] = true

		if it.TargetRepsMin != nil && it.TargetRepsMax != nil && *it.TargetRepsMin > *it.TargetRepsMax {
			return fmt.Sprintf("items[%d] のレップ範囲が逆: %d〜%d", i, *it.TargetRepsMin, *it.TargetRepsMax)
		}
	}

	return ""
}

func toTemplateInput(in openapi.TemplateInput) repository.TemplateInput {
	return repository.TemplateInput{ID: in.Id, Name: in.Name, Items: in.Items}
}

func templateBadRequest(title, detail string) openapi.CreateTemplate400ApplicationProblemPlusJSONResponse {
	return openapi.CreateTemplate400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}

func updateTemplateBadRequest(title, detail string) openapi.UpdateTemplate400ApplicationProblemPlusJSONResponse {
	return openapi.UpdateTemplate400ApplicationProblemPlusJSONResponse{
		BadRequestApplicationProblemPlusJSONResponse: openapi.BadRequestApplicationProblemPlusJSONResponse(
			problem(400, title, detail)),
	}
}
