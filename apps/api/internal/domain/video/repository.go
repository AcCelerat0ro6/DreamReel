package domainvideo

import "context"

// Repository 定义视频领域需要的持久化能力。
type Repository interface {
	// Save 保存视频。
	Save(ctx context.Context, video *Video) error
	// FindByAuthorAndIdempotencyKey 查询作者和幂等键的视频，用于创建视频时避免重复创建。
	FindByAuthorAndIdempotencyKey(ctx context.Context, authorID int64, idempotencyKey string) (*Video, error)
}
