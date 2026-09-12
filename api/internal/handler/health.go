package handler

import (
	"context"
	"log/slog"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/timeutil"
)

// GetHealth は DB 接続を含む疎通確認を返す。
//
// Supabase Free は7日間アクセスがないとプロジェクトを停止するため、
// このエンドポイントを定期的に叩いて keepalive を兼ねる（docs/05-インフラ設計 §2.1）。
// DB まで到達しないと keepalive にならないので、プロセスの生存だけを見る実装にはしない。
func (s *Server) GetHealth(ctx context.Context, _ openapi.GetHealthRequestObject) (openapi.GetHealthResponseObject, error) {
	if err := s.db.Ping(ctx); err != nil {
		slog.ErrorContext(ctx, "DB への疎通に失敗", slog.Any("error", err))
		return openapi.GetHealth503ApplicationProblemPlusJSONResponse{
			ServiceUnavailableApplicationProblemPlusJSONResponse: openapi.ServiceUnavailableApplicationProblemPlusJSONResponse{
				Type:   "about:blank",
				Title:  "データベースに接続できない",
				Status: 503,
			},
		}, nil
	}

	return openapi.GetHealth200JSONResponse{
		Status: openapi.Ok,
		Time:   timeutil.Now(),
	}, nil
}
