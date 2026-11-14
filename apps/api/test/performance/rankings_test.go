//go:build labperf

package performance

import (
	"cmp"
	"reflect"
	"slices"
	"strings"

	video "DreamReel/internal/domain/video"
	repo "DreamReel/internal/infra/persistence/video"
)

// Compare where aggregation happens, keeping joins, ranking weights, tie-breakers
// and the API's extra row for has_more consistent on this ASCII model-name fixture.
func (s *suite) rankings() {
	r := repo.New(s.db)
	const limit = 11
	expected, err := r.ListModelRankings(s.ctx, "hot", limit, 0)
	check(err)
	require(len(expected) == limit, "model leaderboard is incomplete")
	inGo := func(int) {
		var works []video.ModelRanking
		check(s.db.Raw(`SELECT TRIM(v.model_name) AS model_name, COALESCE(vs.like_count,0) AS like_count,
			COALESCE(vs.comment_count,0) AS comment_count, COALESCE(vs.favorite_count,0) AS favorite_count
			FROM video v LEFT JOIN video_stat vs ON vs.video_id=v.id
			WHERE v.status=2 AND v.published_at IS NOT NULL AND TRIM(v.model_name)<>''`).Scan(&works).Error)
		require(len(works) == 10000, "expected 10000 eligible works")
		groups := make(map[string]video.ModelRanking)
		for _, work := range works {
			key := strings.ToLower(work.ModelName)
			item := groups[key]
			if item.VideoCount == 0 || work.ModelName < item.ModelName {
				item.ModelName = work.ModelName
			}
			item.VideoCount++
			item.LikeCount += work.LikeCount
			item.CommentCount += work.CommentCount
			item.FavoriteCount += work.FavoriteCount
			item.HotScore += work.LikeCount*3 + work.CommentCount*5 + work.FavoriteCount*4
			groups[key] = item
		}
		rows := make([]video.ModelRanking, 0, len(groups))
		for _, item := range groups {
			rows = append(rows, item)
		}
		slices.SortFunc(rows, func(a, b video.ModelRanking) int {
			return cmp.Or(cmp.Compare(b.HotScore, a.HotScore), cmp.Compare(b.VideoCount, a.VideoCount), strings.Compare(a.ModelName, b.ModelName))
		})
		require(reflect.DeepEqual(rows[:limit], expected), "Go ranking differs from SQL")
	}
	inSQL := func(int) {
		rows, err := r.ListModelRankings(s.ctx, "hot", limit, 0)
		check(err)
		require(reflect.DeepEqual(rows, expected), "SQL ranking mismatch")
	}
	s.pair("model_rankings", 100, trial{run: inGo}, trial{run: inSQL})
}
