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
	ErrUpdateUserProfileFailed          = errors.New("failed to update user interest profile")
	ErrMarshalInterestVectorFailed      = errors.New("failed to marshal interest vector")
)

type Service struct {
	repo     domainrecommendation.Repository
	sessions SessionStore
	now      func() time.Time
}

type Option func(*Service)

func New(repo domainrecommendation.Repository, options ...Option) *Service {
	service := &Service{
		repo:     repo,
		sessions: newMemorySessionStore(),
		now:      func() time.Time { return time.Now().UTC().Truncate(time.Minute) },
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

type ExposureInput struct {
	UserID    int64
	VideoID   int64
	Scene     string
	RequestID string
}

type ExposureDecisionInput struct {
	UserID    int64
	Scene     string
	RequestID string
	VideoIDs  []int64
}

type ExposureDecisionResult struct {
	UserID    int64
	Scene     string
	RequestID string
	Decisions []*domainrecommendation.ExposureDecision
}

type ExposureResult struct {
	Exposures []*domainrecommendation.Exposure
}

// RefreshProfileInput 定义观看事件驱动的用户推荐画像刷新入参。
type RefreshProfileInput struct {
	UserID      int64
	VideoID     int64
	EventType   string
	WatchMs     int
	Completed   bool
	ViewEventID int64
}

// RefreshProfileResult 定义用户推荐画像刷新结果。
type RefreshProfileResult struct {
	Profile *domainrecommendation.UserInterestProfile
	Updated bool
}
