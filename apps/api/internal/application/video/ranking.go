package applicationvideo

import (
	"context"
	"errors"
	"strings"

	domainvideo "DreamReel/internal/domain/video"
	"go.uber.org/zap"
)

var ErrInvalidRankingQuery = errors.New("sort must be hot, works or likes; limit must be positive; offset must be between 0 and 10000")
var ErrLoadModelRankings = errors.New("failed to load model rankings")

type RankingService struct{ repo domainvideo.RankingRepository }

type RankingResult struct {
	Items   []domainvideo.ModelRanking
	Sort    string
	Limit   int
	Offset  int
	HasMore bool
}

func NewRanking(repo domainvideo.RankingRepository) *RankingService {
	return &RankingService{repo: repo}
}

func (s *RankingService) List(ctx context.Context, sort string, limit, offset int) (*RankingResult, error) {
	sort = strings.ToLower(strings.TrimSpace(sort))
	if sort == "" {
		sort = "hot"
	}
	if (sort != "hot" && sort != "works" && sort != "likes") || limit <= 0 || offset < 0 || offset > 10000 {
		return nil, ErrInvalidRankingQuery
	}
	if limit > 100 {
		limit = 100
	}
	items, err := s.repo.ListModelRankings(ctx, sort, limit+1, offset)
	if err != nil {
		zap.L().Error("load model rankings failed", zap.Error(err))
		return nil, ErrLoadModelRankings
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	if items == nil {
		items = []domainvideo.ModelRanking{}
	}
	return &RankingResult{Items: items, Sort: sort, Limit: limit, Offset: offset, HasMore: hasMore}, nil
}
