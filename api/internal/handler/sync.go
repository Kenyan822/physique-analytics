package handler

import (
	"context"

	"github.com/Kenyan822/physique-analytics/api/gen/openapi"
	"github.com/Kenyan822/physique-analytics/api/internal/repository"
)

// PullSync は updatedSince 以降の差分を返す（ADR-0014）。
func (s *Server) PullSync(ctx context.Context, req openapi.PullSyncRequestObject) (openapi.PullSyncResponseObject, error) {
	res, err := s.sync.Pull(ctx, req.Params.UpdatedSince)
	if err != nil {
		return nil, err
	}

	return openapi.PullSync200JSONResponse{
		ServerTime: res.ServerTime,
		Exercises:  res.Exercises,
		Sessions:   res.Sessions,
		Sets:       res.Sets,
		Templates:  res.Templates,
	}, nil
}

// PushSync はオフライン中に溜めた変更を一括で適用する。
func (s *Server) PushSync(ctx context.Context, req openapi.PushSyncRequestObject) (openapi.PushSyncResponseObject, error) {
	in := repository.PushInput{}

	if req.Body != nil {
		if req.Body.Sessions != nil {
			for _, v := range *req.Body.Sessions {
				in.Sessions = append(in.Sessions, repository.SessionInput{
					ID:         v.Id,
					Date:       v.Date,
					TemplateID: v.TemplateId,
					Note:       v.Note,
					UpdatedAt:  v.UpdatedAt,
					Sets:       toSetInputs(v.Sets),
				})
			}
		}
		if req.Body.Sets != nil {
			for _, v := range *req.Body.Sets {
				set := toSetInput(v)
				set.SessionID = v.SessionId
				set.UpdatedAt = v.UpdatedAt
				in.Sets = append(in.Sets, set)
			}
		}
	}

	res, err := s.sync.Push(ctx, in)
	if err != nil {
		return nil, err
	}

	out := openapi.PushSync200JSONResponse{
		Applied:    res.Applied,
		ServerTime: res.ServerTime,
	}
	out.Conflicts = make([]openapi.SyncConflict, 0, len(res.Conflicts))
	for _, c := range res.Conflicts {
		out.Conflicts = append(out.Conflicts, openapi.SyncConflict{
			Id:              c.ID,
			Resource:        openapi.SyncConflictResource(c.Resource),
			ServerUpdatedAt: c.ServerUpdatedAt,
		})
	}

	return out, nil
}

func toSetInputs(sets *[]openapi.WorkoutSetInput) []repository.SetInput {
	if sets == nil {
		return nil
	}

	out := make([]repository.SetInput, 0, len(*sets))
	for _, v := range *sets {
		out = append(out, toSetInput(v))
	}

	return out
}
