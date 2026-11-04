package domainrecommendation

import (
	"context"
	"time"
)

type Repository interface {
	// ListCandidatePool 粗粒度召回，筛选该用户当前有资格看到的一批候选视频原始池。
	ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*Candidate, error)
	// LoadVideoVectors 按批次取出视频的内容向量。
	LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error)
	// LoadUserInterestVector 加载用户兴趣向量。
	LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error)
	// ListRecentBehaviorSamples 列出用户近期的观看视频行为样本。
	ListRecentBehaviorSamples(ctx context.Context, userID int64, model string, since time.Time, limit int) ([]*BehaviorSample, error)
	// SaveUserInterestProfile 保存用户兴趣向量。
	SaveUserInterestProfile(ctx context.Context, profile *UserInterestProfile) error
	// ListRecentExposures 查询用户在指定时间窗口内已曝光过的视频记录。
	ListRecentExposures(ctx context.Context, userID int64, videoIDs []int64, since time.Time) ([]*Exposure, error)
	// SaveExposures 批量写入曝光流水并更新曝光聚合表。
	SaveExposures(ctx context.Context, exposures []*ExposureWrite) ([]*Exposure, error)
}
