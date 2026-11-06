package domainvideo

import "context"

// ModelRanking aggregates the public works attributed to a generation model.
type ModelRanking struct {
	ModelName     string
	VideoCount    int64
	LikeCount     int64
	CommentCount  int64
	FavoriteCount int64
	HotScore      int64
}

type RankingRepository interface {
	ListModelRankings(ctx context.Context, sort string, limit, offset int) ([]ModelRanking, error)
}
