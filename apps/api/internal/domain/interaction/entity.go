package domaininteraction

const (
	ActionTypeLike     = "LIKE"
	ActionTypeFavorite = "FAVORITE"

	ActionStatusActive   = 1
	ActionStatusCanceled = 2

	CommentStatusNormal  = 1
	CommentStatusDeleted = 2

	MaxCommentContentLength = 1000
	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)

// VideoStat 保存互动模块需要的视频统计快照。
type VideoStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}
