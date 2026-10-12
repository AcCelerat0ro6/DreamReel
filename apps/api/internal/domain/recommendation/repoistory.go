package domainrecommendation

import (
	"context"
)

type Repository interface {
	// ListCandidatePool 粗粒度召回, 筛选该用户当前有资格看到的一批候选视频原始池
	ListCandidatePool(ctx context.Context, userID int64, limit int) ([]*Candidate, error)
	// LoadVideoVectors 按批次取出视频的内容向量
	LoadVideoVectors(ctx context.Context, videoIDs []int64) (map[int64][]float64, error)
	// LoadUserInterestVector 加载用户兴趣向量
	LoadUserInterestVector(ctx context.Context, userID int64) ([]float64, bool, error)
}
