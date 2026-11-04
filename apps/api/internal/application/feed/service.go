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

// WithStrategy 注册一个额外 Feed 策略。
func WithStrategy(strategy Strategy) Option {
	return func(s *Service) {
		s.RegisterStrategy(strategy)
	}
}

// RegisterStrategy 把 scene 和具体策略绑定，新增 Feed 类型时在装配层调用。
func (s *Service) RegisterStrategy(strategy Strategy) {
	if strategy == nil {
		return
	}
	scene := domainfeed.NormalizeScene(strategy.Scene())
	s.strategies[scene] = strategy
}

// GetFeed 根据 scene 选择策略并返回分页结果。
func (s *Service) GetFeed(ctx context.Context, request FeedRequest) (*FeedResult, error) {
	start := time.Now()
	request.Scene = domainfeed.NormalizeScene(request.Scene)
	if request.Scene == "" {
		request.Scene = s.defaultScene
	}

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

// GetTimelineFeed 使用 cursor+limit 读取时间线 Feed。
func (s *Service) GetTimelineFeed(ctx context.Context, cursor string, limit int) (*FeedResult, error) {
	return s.GetFeed(ctx, FeedRequest{
		Scene:  domainfeed.SceneTimeline,
		Cursor: cursor,
		Limit:  limit,
	})
}

// RefreshFeed 从第一页重新加载默认 Feed，适合下拉刷新场景。
func (s *Service) RefreshFeed(ctx context.Context, limit int) (*FeedResult, error) {
	return s.GetTimelineFeed(ctx, "", limit)
}
