package applicationfeed

import (
	applicationrecommendation "DreamReel/internal/application/recommendation"
	domainfeed "DreamReel/internal/domain/feed"
	"context"
	"errors"

	"go.uber.org/zap"
)

// RecommendStrategy 使用推荐策略返回推荐流
type RecommendStrategy struct {
	repo        domainfeed.Repository
	cache       FeedCache
	recommender Recommender
}

type Recommender interface {
	Recommend(ctx context.Context, input applicationrecommendation.CandidateRequest) (*applicationrecommendation.CandidateResult, error)
}

// NewRecommendStrategy 创建推荐 Feed 策略。
func NewRecommendStrategy(repo domainfeed.Repository, recommender Recommender) *RecommendStrategy {
	return &RecommendStrategy{
		repo:        repo,
		recommender: recommender,
	}
}

func WithRecommender(recommender Recommender) Option {
	return func(s *Service) {
		s.RegisterStrategy(NewRecommendStrategy(s.repo, recommender))
	}
}

// Scene 返回推荐场景。
func (s *RecommendStrategy) Scene() domainfeed.Scene {
	return domainfeed.SceneRecommend
}

// List 读取推荐候选，并按推荐服务给出的顺序组装 Feed 卡片。
func (s *RecommendStrategy) List(ctx context.Context, req FeedRequest) (*FeedResult, error) {
	if req.ViewerID <= 0 {
		return nil, domainfeed.ErrViewerRequired
	}
	limit := normalizeLimit(req.Limit)
	result, err := s.recommender.Recommend(ctx, applicationrecommendation.CandidateRequest{
		UserID:    req.ViewerID,
		Scene:     string(domainfeed.SceneRecommend),
		RequestID: clientContextValue(req.ClientContext, "request_id"),
		Cursor:    req.Cursor,
		Limit:     limit,
	})
	if err != nil {
		if errors.Is(err, applicationrecommendation.ErrLoadRecommendationFailed) {
			return nil, ErrLoadFeedFailed
		}
		return nil, err
	}
	pageItems := make([]*domainfeed.FeedPageItem, 0, len(result.Candidates))
	for _, candidate := range result.Candidates {
		pageItems = append(pageItems, &domainfeed.FeedPageItem{
			VideoID:     candidate.VideoID,
			AuthorID:    candidate.AuthorID,
			PublishedAt: candidate.PublishedAt,
			HotScore:    candidate.HotScore,
		})
	}
	items, err := assembleFeedItems(ctx, s.repo, s.cache, pageItems, req.ViewerID)
	if err != nil {
		zap.L().Error("failed to assemble recommend feed items", zap.Int64("viewer_id", req.ViewerID), zap.Error(err))
		return nil, ErrLoadFeedFailed
	}
	return &FeedResult{
		Scene:      domainfeed.SceneRecommend,
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}, nil
}
