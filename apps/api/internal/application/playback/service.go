package applicationplayback

import (
	domainplayback "DreamReel/internal/domain/playback"
	"context"
	"errors"
	"strings"
)

var (
	ErrLoadPlaybackFailed  = errors.New("failed to load playback data")
	ErrSaveQoSReportFailed = errors.New("failed to save playback qos report")
)

type Service struct {
	repo domainplayback.Repository
}

func New(repo domainplayback.Repository) *Service {
	return &Service{repo: repo}
}

type ConfigResult struct {
	Config *domainplayback.Config
}

// GetConfig 查询端侧播放配置，配置缺失时返回领域默认值。
func (s *Service) GetConfig(ctx context.Context, platform string, networkType string) (*ConfigResult, error) {
	platform = domainplayback.NormalizePlatform(platform)
	networkType = domainplayback.NormalizeNetworkType(networkType)
	if len(platform) > domainplayback.MaxPlatformLength {
		return nil, domainplayback.ErrInvalidPlatform
	}
	if len(networkType) > domainplayback.MaxNetworkTypeLength {
		return nil, domainplayback.ErrInvalidNetworkType
	}

	config, err := s.repo.FindConfig(ctx, platform, networkType)
	if err != nil {
		return nil, ErrLoadPlaybackFailed
	}
	// 具体网络类型未单独配置时，回退查找该平台下的 DEFAULT 默认网络配置
	if config == nil && networkType != domainplayback.NetworkDefault {
		config, err = s.repo.FindConfig(ctx, platform, domainplayback.NetworkDefault)
		if err != nil {
			return nil, ErrLoadPlaybackFailed
		}
	}
	if config == nil {
		config = domainplayback.DefaultConfig(platform, networkType)
	}
	return &ConfigResult{Config: normalizeConfig(config, platform, networkType)}, nil

}

// ListPreloadVideos 查询当前视频后续资源，供端侧预加载。
func (s *Service) ListPreloadVideos(ctx context.Context, currentVideoID int64, limit int) (*PreloadResult, error) {
	if currentVideoID < 0 {
		return nil, domainplayback.ErrInvalidVideoID
	}
	limit = normalizeLimit(limit)

	items, err := s.repo.ListPreloadVideos(ctx, currentVideoID, limit)
	if err != nil {
		return nil, ErrLoadPlaybackFailed
	}
	return &PreloadResult{Items: items}, nil
}

// CreateQoSReport 写入播放质量流水。
func (s *Service) CreateQoSReport(ctx context.Context, userID int64, videoID int64, firstFrameMs *int, stutterCount int, watchMs int, idempotencyKey string) (*QoSReportResult, error) {
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	report, err := domainplayback.NewQoSReport(userID, videoID, firstFrameMs, stutterCount, watchMs, idempotencyKey)
	if err != nil {
		return nil, err
	}
	created, inserted, err := s.repo.CreateQoSReport(ctx, report)
	if err != nil {
		return nil, ErrSaveQoSReportFailed
	}
	return &QoSReportResult{Report: created, Created: inserted}, nil
}
