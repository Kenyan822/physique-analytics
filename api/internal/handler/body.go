package handler

import (
	"context"
	"fmt"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// ListDailyMetrics は日次記録の一覧を返す。
func (s *Server) ListDailyMetrics(ctx context.Context, req openapi.ListDailyMetricsRequestObject) (openapi.ListDailyMetricsResponseObject, error) {
	items, err := s.body.ListDaily(ctx, req.Params.From, req.Params.To)
	if err != nil {
		return nil, err
	}
	// nil のままだと JSON が items: null になり、required: [items] を満たさない
	if items == nil {
		items = []openapi.DailyMetrics{}
	}

	return openapi.ListDailyMetrics200JSONResponse{Items: items}, nil
}

// GetDailyMetrics は1日分の記録を返す。
func (s *Server) GetDailyMetrics(ctx context.Context, req openapi.GetDailyMetricsRequestObject) (openapi.GetDailyMetricsResponseObject, error) {
	d, err := s.body.GetDaily(ctx, req.Date)
	if repository.IsNotFound(err) {
		return openapi.GetDailyMetrics404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "その日の記録が無い", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.GetDailyMetrics200JSONResponse(d), nil
}

// PutDailyMetrics は日付をキーに日次記録を upsert する。
func (s *Server) PutDailyMetrics(ctx context.Context, req openapi.PutDailyMetricsRequestObject) (openapi.PutDailyMetricsResponseObject, error) {
	if req.Body == nil {
		return dailyValidationFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateDailyInput(*req.Body); msg != "" {
		return dailyValidationFailed(field, msg), nil
	}

	d, err := s.body.PutDaily(ctx, toDailyInput(*req.Body))
	if err != nil {
		return nil, err
	}

	return openapi.PutDailyMetrics200JSONResponse(d), nil
}

// DeleteDailyMetrics は1日分を論理削除する。
func (s *Server) DeleteDailyMetrics(ctx context.Context, req openapi.DeleteDailyMetricsRequestObject) (openapi.DeleteDailyMetricsResponseObject, error) {
	err := s.body.SoftDeleteDaily(ctx, req.Date)
	if repository.IsNotFound(err) {
		return openapi.DeleteDailyMetrics404ApplicationProblemPlusJSONResponse{
			NotFoundApplicationProblemPlusJSONResponse: openapi.NotFoundApplicationProblemPlusJSONResponse(
				problem(404, "その日の記録が無い", "")),
		}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.DeleteDailyMetrics204Response{}, nil
}

// ListMeasurements は周囲長の一覧を返す。
func (s *Server) ListMeasurements(ctx context.Context, req openapi.ListMeasurementsRequestObject) (openapi.ListMeasurementsResponseObject, error) {
	items, err := s.body.ListMeasurements(ctx, req.Params.From, req.Params.To)
	if err != nil {
		return nil, err
	}
	if items == nil {
		items = []openapi.BodyMeasurement{}
	}

	return openapi.ListMeasurements200JSONResponse{Items: items}, nil
}

// PutMeasurement は日付をキーに周囲長を upsert する（要件 B-02）。
func (s *Server) PutMeasurement(ctx context.Context, req openapi.PutMeasurementRequestObject) (openapi.PutMeasurementResponseObject, error) {
	if req.Body == nil {
		return measurementValidationFailed("body", "リクエストボディが無い"), nil
	}
	if field, msg := validateMeasurementInput(*req.Body); msg != "" {
		return measurementValidationFailed(field, msg), nil
	}

	m, err := s.body.PutMeasurement(ctx, toMeasurementInput(*req.Body))
	if err != nil {
		return nil, err
	}

	return openapi.PutMeasurement200JSONResponse(m), nil
}

// GetLatestMeasurement は直近の周囲長を返す（要件 B-03）。
//
// **記録が無くても 404 にしない。** 初回は無いのが正常で、
// エラーにすると Web の入力画面が開けなくなる。
func (s *Server) GetLatestMeasurement(ctx context.Context, _ openapi.GetLatestMeasurementRequestObject) (openapi.GetLatestMeasurementResponseObject, error) {
	m, err := s.body.LatestMeasurement(ctx)
	if repository.IsNotFound(err) {
		return openapi.GetLatestMeasurement200JSONResponse{}, nil
	}
	if err != nil {
		return nil, err
	}

	return openapi.GetLatestMeasurement200JSONResponse{Measurement: &m}, nil
}

// validateDailyInput は DB の check 制約と同じ範囲をハンドラ側で見る。
//
// DB に任せると 23514 が 500 になって「何が悪いか」が返らない。
// 範囲は api/migrations/000003 の check と合わせてある。
func validateDailyInput(in openapi.DailyMetricsInput) (field, message string) {
	checks := []struct {
		field  string
		v      *float32
		lo, hi float32
		// minExclusive は「0 より大きい」を表す。体重 0kg は入力ミス
		minExclusive bool
	}{
		{field: "weightKg", v: in.WeightKg, lo: 0, hi: 300, minExclusive: true},
		{field: "bodyfatPct", v: in.BodyfatPct, lo: 0, hi: 70},
		{field: "sleepH", v: in.SleepH, lo: 0, hi: 24},
	}
	for _, c := range checks {
		if msg := checkFloat(c.v, c.lo, c.hi, c.minExclusive); msg != "" {
			return c.field, msg
		}
	}

	intChecks := []struct {
		field        string
		v            *int
		lo, hi       int
		minExclusive bool
	}{
		{field: "kcal", v: in.Kcal, lo: 0, hi: 20000},
		{field: "proteinG", v: in.ProteinG, lo: 0, hi: 1000},
		{field: "fatG", v: in.FatG, lo: 0, hi: 1000},
		{field: "carbG", v: in.CarbG, lo: 0, hi: 2000},
		{field: "steps", v: in.Steps, lo: 0, hi: 200000},
		{field: "fatigue", v: in.Fatigue, lo: 1, hi: 5},
		{field: "hrvMs", v: in.HrvMs, lo: 0, hi: 500, minExclusive: true},
		{field: "restingHr", v: in.RestingHr, lo: 0, hi: 200, minExclusive: true},
		{field: "deepSleepMin", v: in.DeepSleepMin, lo: 0, hi: 1440},
	}
	for _, c := range intChecks {
		if msg := checkInt(c.v, c.lo, c.hi, c.minExclusive); msg != "" {
			return c.field, msg
		}
	}

	return "", ""
}

// validateMeasurementInput は周囲長の範囲を見る。上限は openapi.yaml と揃える。
func validateMeasurementInput(in openapi.BodyMeasurementInput) (field, message string) {
	checks := []struct {
		field string
		v     *float32
		hi    float32
	}{
		{"neckCm", in.NeckCm, 100},
		{"shoulderCm", in.ShoulderCm, 200},
		{"chestCm", in.ChestCm, 200},
		{"waistNavelCm", in.WaistNavelCm, 200},
		{"hipCm", in.HipCm, 200},
		{"armRCm", in.ArmRCm, 100},
		{"thighRCm", in.ThighRCm, 150},
		{"calfRCm", in.CalfRCm, 100},
	}
	for _, c := range checks {
		if msg := checkFloat(c.v, 0, c.hi, true); msg != "" {
			return c.field, msg
		}
	}

	return "", ""
}

func checkFloat(v *float32, lo, hi float32, minExclusive bool) string {
	if v == nil {
		return ""
	}
	if minExclusive && *v <= lo {
		return fmt.Sprintf("%g より大きい値にする", lo)
	}
	if !minExclusive && *v < lo {
		return fmt.Sprintf("%g 以上にする", lo)
	}
	if *v > hi {
		return fmt.Sprintf("%g 以下にする", hi)
	}

	return ""
}

func checkInt(v *int, lo, hi int, minExclusive bool) string {
	if v == nil {
		return ""
	}
	if minExclusive && *v <= lo {
		return fmt.Sprintf("%d より大きい値にする", lo)
	}
	if !minExclusive && *v < lo {
		return fmt.Sprintf("%d 以上にする", lo)
	}
	if *v > hi {
		return fmt.Sprintf("%d 以下にする", hi)
	}

	return ""
}

func toDailyInput(in openapi.DailyMetricsInput) repository.DailyInput {
	return repository.DailyInput{
		Date:         in.Date,
		WeightKg:     in.WeightKg,
		BodyfatPct:   in.BodyfatPct,
		Kcal:         in.Kcal,
		ProteinG:     in.ProteinG,
		FatG:         in.FatG,
		CarbG:        in.CarbG,
		SleepH:       in.SleepH,
		Steps:        in.Steps,
		Fatigue:      in.Fatigue,
		HrvMs:        in.HrvMs,
		RestingHr:    in.RestingHr,
		DeepSleepMin: in.DeepSleepMin,
		Note:         in.Note,
	}
}

func toMeasurementInput(in openapi.BodyMeasurementInput) repository.MeasurementInput {
	return repository.MeasurementInput{
		Date:         in.Date,
		NeckCm:       in.NeckCm,
		ShoulderCm:   in.ShoulderCm,
		ChestCm:      in.ChestCm,
		WaistNavelCm: in.WaistNavelCm,
		HipCm:        in.HipCm,
		ArmRCm:       in.ArmRCm,
		ThighRCm:     in.ThighRCm,
		CalfRCm:      in.CalfRCm,
	}
}

func dailyValidationFailed(field, message string) openapi.PutDailyMetrics422ApplicationProblemPlusJSONResponse {
	return openapi.PutDailyMetrics422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func measurementValidationFailed(field, message string) openapi.PutMeasurement422ApplicationProblemPlusJSONResponse {
	return openapi.PutMeasurement422ApplicationProblemPlusJSONResponse{
		ValidationFailedApplicationProblemPlusJSONResponse: openapi.ValidationFailedApplicationProblemPlusJSONResponse(
			validationProblem(field, message)),
	}
}

func validationProblem(field, message string) openapi.Problem {
	p := problem(422, "入力が仕様に合わない", message)
	p.Errors = &[]struct {
		Field   string `json:"field"`
		Message string `json:"message"`
	}{{Field: field, Message: message}}

	return p
}
