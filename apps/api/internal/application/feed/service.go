package applicationfeed

import (
	domainfeed "DreamReel/internal/domain/feed"
	metrics "DreamReel/internal/infra/metrics"
	"context"
	"time"
)

type Service struct {
	repo         domainfeed.Repository
	strategies   map[domainfeed.Scene]Strategy
	defaultScene domainfeed.Scene
}

// Option 用于在装配阶段注册额外 Feed 策略。
type Option func(*Service)

func (s *Service) GetFeed(ctx context.Context, request FeedRequest) (*FeedResult, error) {
	start := time.Now()
	request.Scene = domainfeed.NormalizeScene(request.Scene)
	strategy, ok := s.strategies[request.Scene]
	if !ok {
		metrics.ObserveFeed(string(request.Scene), time.Since(start), 0, domainfeed.ErrUnsupportedScene)
		return nil, domainfeed.ErrUnsupportedScene
	}
	result, err := strategy.List(ctx, request)
	itemCount := 0
	if result != nil {
		itemCount = len(result.Items)
	}
	metrics.ObserveFeed(string(request.Scene), time.Since(start), itemCount, err)
	return result, err
}
