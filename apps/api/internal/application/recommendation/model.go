package applicationrecommendation

import (
	domainrecommendation "DreamReel/internal/domain/recommendation"
	"errors"
	"time"
)

var (
	ErrLoadRecommendationFailed         = errors.New("failed to load recommendations")
	ErrLoadExposureDecisionsFailed      = errors.New("failed to load exposure decisions")
	ErrSaveRecommendationExposureFailed = errors.New("failed to save recommendation exposure")
)

type Service struct {
	repo domainrecommendation.Repository
	now  func() time.Time
}

type Option func(*Service)

func New(repo domainrecommendation.Repository, options ...Option) *Service {
	service := &Service{
		repo: repo,
		now:  func() time.Time { return time.Now().UTC().Truncate(time.Minute) },
	}
	for _, option := range options {
		option(service)
	}
	return service
}

type CandidateRequest struct {
	UserID    int64
	Scene     string
	RequestID string
	Cursor    string
	Limit     int
}

type CandidateResult struct {
	UserID     int64
	Scene      string
	RequestID  string
	Candidates []*domainrecommendation.Candidate
	NextCursor string
	HasMore    bool
}

type cursorPayload struct {
	RankScore   float64 `json:"rank_score"`
	PublishedAt string  `json:"published_at"`
	VideoID     int64   `json:"video_id"`
}
