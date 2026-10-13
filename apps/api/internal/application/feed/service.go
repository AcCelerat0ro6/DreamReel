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

// New 初始化 Feed Service
func New(repo domainfeed.Repository, options ...Option) *Service {
	service := &Service{
		repo:         repo,
		strategies:   map[domainfeed.Scene]Strategy{},
		defaultScene: domainfeed.DefaultScene,
	}
	service.RegisterStrategy(NewTimelineStrategy(domainfeed.SceneTimeline, repo))
	service.RegisterStrategy(NewHotStrategy(repo))
	service.RegisterStrategy(NewFollowingStrategy(repo))
	for _, option := range options {
		option(service)
	}
	return service
}

// Option 用于在装配阶段注册额外 Feed 策略。
type Option func(*Service)

// RegisterStrategy 把 scene 和具体策略绑定，新增 Feed 类型时在装配层调用。
func (s *Service) RegisterStrategy(strategy Strategy) {
	if strategy == nil {
		return
	}
	scene := domainfeed.NormalizeScene(strategy.Scene())
	s.strategies[scene] = strategy
}

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
