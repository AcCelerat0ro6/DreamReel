package domainrecommendation

import (
	"math"
	"strings"
	"time"
)

const (
	MaxLimit             = 100
	MaxSceneLength       = 32
	MaxRequestIDLength   = 64
	RecentExposureWindow = 7 * 24 * time.Hour

	ExposureDecisionReasonFresh           = "fresh"
	ExposureDecisionReasonRecentlyExposed = "recently_exposed"
)

type Candidate struct {
	VideoID        int64
	AuthorID       int64
	RankScore      float64
	Similarity     float64
	HotScore       int
	FreshnessScore float64
	Reason         string
	PublishedAt    time.Time
}

type Cursor struct {
	RankScore   float64
	PublishedAt time.Time
	VideoID     int64
}

func (c *Cursor) Valid() bool {
	return c != nil && c.VideoID > 0 && !c.PublishedAt.IsZero() && !math.IsNaN(c.RankScore) && !math.IsInf(c.RankScore, 0)
}

type CandidateRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	Cursor    *Cursor
	Limit     int
}

// NewCandidateRequest 转换领域模型的同时，校验相应字段是否正确
func NewCandidateRequest(userID int64, scene string, requestID string, cursor *Cursor, limit int) (*CandidateRequest, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	scene = strings.TrimSpace(strings.ToLower(scene))
	requestID = strings.TrimSpace(requestID)
	if scene == "" {
		return nil, ErrEmptyScene
	}
	if len(scene) > MaxSceneLength {
		return nil, ErrSceneTooLong
	}
	if len(requestID) > MaxRequestIDLength {
		return nil, ErrRequestIDTooLong
	}
	if limit <= 0 || limit > MaxLimit {
		return nil, ErrInvalidLimit
	}
	if cursor != nil && !cursor.Valid() {
		return nil, ErrInvalidCursor
	}
	return &CandidateRequest{
		UserID:    userID,
		Scene:     scene,
		RequestID: requestID,
		Cursor:    cursor,
		Limit:     limit,
	}, nil
}

func RestoreCandidate(videoID int64, authorID int64, rankScore float64, similarity float64, hotScore int, freshnessScore float64, reason string, publishedAt time.Time) *Candidate {
	return &Candidate{
		VideoID:        videoID,
		AuthorID:       authorID,
		RankScore:      rankScore,
		Similarity:     similarity,
		HotScore:       hotScore,
		FreshnessScore: freshnessScore,
		Reason:         strings.TrimSpace(reason),
		PublishedAt:    publishedAt,
	}
}
