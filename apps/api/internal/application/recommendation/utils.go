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

const (
	defaultLimit            = 10
	candidatePoolMultiplier = 8
	minCandidatePoolSize    = 50
	maxCandidatePoolSize    = 500
)

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

	var payload cursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	publishedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.PublishedAt))
	if err != nil {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	cursor := &domainrecommendation.Cursor{
		RankScore:   payload.RankScore,
		PublishedAt: publishedAt,
		VideoID:     payload.VideoID,
	}
	if !cursor.Valid() {
		return nil, domainrecommendation.ErrInvalidCursor
	}
	return cursor, nil

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

func candidatePoolLimit(limit int) int {
	poolLimit := limit * candidatePoolMultiplier
	if poolLimit < minCandidatePoolSize {
		poolLimit = minCandidatePoolSize
	}
	if poolLimit > maxCandidatePoolSize {
		poolLimit = maxCandidatePoolSize
	}
	return poolLimit
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

func sameScore(left float64, right float64) bool {
	return math.Abs(left-right) < 0.000000001
}

func filterByCursor(candidates []*domainrecommendation.Candidate, cursor *domainrecommendation.Cursor) []*domainrecommendation.Candidate {
	if cursor == nil {
		return candidates
	}
	filtered := make([]*domainrecommendation.Candidate, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if candidate.RankScore < cursor.RankScore ||
			(sameScore(candidate.RankScore, cursor.RankScore) && candidate.PublishedAt.Before(cursor.PublishedAt)) ||
			(sameScore(candidate.RankScore, cursor.RankScore) && candidate.PublishedAt.Equal(cursor.PublishedAt) && candidate.VideoID < cursor.VideoID) {
			filtered = append(filtered, candidate)
		}
	}
	return filtered
}

func encodeCursor(cursor *domainrecommendation.Cursor) string {
	if cursor == nil || !cursor.Valid() {
		return ""
	}
	content, err := json.Marshal(cursorPayload{
		RankScore:   cursor.RankScore,
		PublishedAt: cursor.PublishedAt.UTC().Format(time.RFC3339Nano),
		VideoID:     cursor.VideoID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}

// interleaveByAuthor 对候选视频进行同作者打散
func interleaveByAuthor(candidates []*domainrecommendation.Candidate) []*domainrecommendation.Candidate {
	if len(candidates) <= 2 {
		return candidates
	}
	output := make([]*domainrecommendation.Candidate, 0, len(candidates))
	delayed := make([]*domainrecommendation.Candidate, 0)
	var previousAuthorID int64
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if previousAuthorID != 0 && candidate.AuthorID == previousAuthorID {
			delayed = append(delayed, candidate)
			continue
		}
		output = append(output, candidate)
		previousAuthorID = candidate.AuthorID
	}
	for len(delayed) > 0 {
		progressed := false
		remaining := make([]*domainrecommendation.Candidate, 0)
		for _, candidate := range delayed {
			if len(output) > 0 && output[len(output)-1].AuthorID == candidate.AuthorID {
				remaining = append(remaining, candidate)
				continue
			}
			output = append(output, candidate)
			progressed = true
		}
		if !progressed {
			output = append(output, remaining...)
			break
		}
		delayed = remaining
	}
	return output
}
