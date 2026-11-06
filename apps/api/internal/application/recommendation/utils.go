package applicationrecommendation

import (
	domainrecommendation "DreamReel/internal/domain/recommendation"
	"encoding/base64"
	"encoding/json"
	"math"
	"sort"
	"strings"
	"time"
)

const defaultLimit = 10

func WithNow(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

func parseCursor(raw string) (*domainrecommendation.Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainrecommendation.ErrInvalidCursor
		}
	}

	var cursor domainrecommendation.Cursor
	if err := json.Unmarshal(content, &cursor); err != nil {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	// 旧版本游标只记录排名分数，升级后由客户端刷新以建立新会话。
	if cursor.SessionID == "" {
		var legacy struct {
			RankScore   *float64  `json:"rank_score"`
			VideoID     int64     `json:"video_id"`
			PublishedAt time.Time `json:"published_at"`
		}
		if json.Unmarshal(content, &legacy) == nil && legacy.RankScore != nil && legacy.VideoID > 0 && !legacy.PublishedAt.IsZero() {
			return nil, domainrecommendation.ErrCursorExpired
		}
	}
	if !cursor.Valid() {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	return &cursor, nil

}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultLimit
	}
	if limit > domainrecommendation.MaxLimit {
		return domainrecommendation.MaxLimit
	}
	return limit
}

// freshnessScore 计算 视频新鲜度得分 区间为[0, 1]
func freshnessScore(now time.Time, publishedAt time.Time) float64 {
	if publishedAt.IsZero() {
		return 0
	}
	hours := now.Sub(publishedAt).Hours()
	if hours < 0 {
		hours = 0
	}
	return 1 / (1 + hours/72)
}

// rankScore 计算 视频最终排名得分 区间为[0, 1]
func rankScore(similarity float64, hotScore int, freshness float64, hasUserVector bool) float64 {
	hot := math.Log1p(float64(maxInt(hotScore, 0))) / 10
	if hasUserVector {
		return similarity*0.70 + hot*0.20 + freshness*0.10
	}
	return hot*0.65 + freshness*0.35
}

func maxInt(left int, right int) int {
	if left > right {
		return left
	}
	return right
}

// recommendationReason 根据相似度得分和热度得分推断推荐原因
func recommendationReason(hasUserVector bool, similarity float64, hotScore int) string {
	// 相似度推荐
	if hasUserVector && similarity > 0.05 {
		return "interest_match"
	}
	// 热度推荐
	if hotScore > 0 {
		return "hot"
	}
	// 新鲜度推荐
	return "fresh"
}

// sortCandidates 对候选视频进行排序，优先级：排名得分 > 视频发布时间
func sortCandidates(candidates []*domainrecommendation.Candidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		left := candidates[i]
		right := candidates[j]
		if left.RankScore != right.RankScore {
			return left.RankScore > right.RankScore
		}
		if !left.PublishedAt.Equal(right.PublishedAt) {
			return left.PublishedAt.After(right.PublishedAt)
		}
		return left.VideoID > right.VideoID
	})
}

func encodeCursor(cursor *domainrecommendation.Cursor) string {
	if cursor == nil || !cursor.Valid() {
		return ""
	}
	content, err := json.Marshal(cursor)
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// interleaveByAuthor 在同一批候选中优先选择不同作者，补池时接续上一批的末位作者。
func interleaveByAuthor(candidates []*domainrecommendation.Candidate, lastAuthor int64) []*domainrecommendation.Candidate {
	result := make([]*domainrecommendation.Candidate, 0, len(candidates))
	for len(candidates) > 0 {
		index := 0
		for index < len(candidates) && candidates[index].AuthorID == lastAuthor {
			index++
		}
		if index == len(candidates) {
			return append(result, candidates...)
		}
		next := candidates[index]
		result = append(result, next)
		lastAuthor = next.AuthorID
		candidates = append(candidates[:index], candidates[index+1:]...)
	}
	return result
}
