package domainrecommendation

import (
	"math"
	"strings"
	"time"
)

const (
	MaxLimit               = 100
	MaxSceneLength         = 32
	MaxRequestIDLength     = 64
	RecentExposureWindow   = 7 * 24 * time.Hour
	PositiveBehaviorWindow = 30 * 24 * time.Hour
	MaxProfileSampleEvents = 200

	BehaviorEventExposed                  = "exposed"
	BehaviorEventPlay                     = "play"
	BehaviorEventComplete                 = "complete"
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

// UserInterestProfile 保存一个用户聚合后的推荐兴趣向量。
type UserInterestProfile struct {
	UserID        int64
	Model         string
	Dimension     int
	Embedding     []float64
	EmbeddingJSON string
	SampleCount   int
	LastEventID   int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// IsProfileTriggerEvent 判断观看事件是否属于触发推荐画像更新的正反馈行为。
func IsProfileTriggerEvent(eventType string, completed bool) bool {
	normalized := strings.TrimSpace(strings.ToLower(eventType))
	if normalized == BehaviorEventExposed {
		return false
	}
	if completed {
		return true
	}
	switch normalized {
	case BehaviorEventPlay, BehaviorEventComplete:
		return true
	default:
		return false
	}

}

// BehaviorSample 保存一条参与画像聚合的观看行为及对应视频向量。
type BehaviorSample struct {
	EventType string
	WatchMs   int
	Completed bool
	Embedding []float64
}

// AggregateInterestVector 按行为权重对视频向量做加权平均，返回用户兴趣向量与有效样本数。
func AggregateInterestVector(samples []*BehaviorSample) ([]float64, int) {
	var sum []float64
	var totalWeight float64
	var sampleCount int
	for _, sample := range samples {
		if sample == nil || !IsValidInterestVector(sample.Embedding) {
			continue
		}
		if !IsProfileTriggerEvent(sample.EventType, sample.Completed) {
			continue
		}
		if len(sum) == 0 {
			sum = make([]float64, len(sample.Embedding))
		}
		if len(sample.Embedding) != len(sum) {
			continue
		}
		weight := BehaviorEventWeight(sample.EventType, sample.WatchMs, sample.Completed)
		if weight <= 0 {
			continue
		}
		for i := range sample.Embedding {
			sum[i] += sample.Embedding[i] * weight
		}
		totalWeight += weight
		sampleCount++
	}
	if len(sum) == 0 || totalWeight == 0 || sampleCount == 0 {
		return nil, 0
	}
	for i := range sum {
		sum[i] = sum[i] / totalWeight
	}
	if !IsValidInterestVector(sum) {
		return nil, 0
	}
	return sum, sampleCount
}

// IsValidInterestVector 校验兴趣向量非空、数值有限且至少包含一个非零特征分量。
func IsValidInterestVector(vector []float64) bool {
	if len(vector) == 0 {
		return false
	}
	hasNonZero := false
	for _, value := range vector {
		if math.IsNaN(value) || math.IsInf(value, 0) {
			return false
		}
		if value != 0 {
			hasNonZero = true
		}
	}
	return hasNonZero
}

// BehaviorEventWeight 根据观看事件类型、时长与完播状态计算兴趣权重。
func BehaviorEventWeight(eventType string, watchMs int, completed bool) float64 {
	switch strings.TrimSpace(strings.ToLower(eventType)) {
	case BehaviorEventComplete:
		return 3
	case BehaviorEventPlay:
		if watchMs < 0 {
			watchMs = 0
		}
		weight := 1 + float64(watchMs)/30000
		if weight > 2 {
			weight = 2
		}
		if completed {
			weight += 1
		}
		return weight
	default:
		if completed {
			return 2
		}
		return 1
	}
}

// NewUserInterestProfile 创建用户推荐兴趣向量领域对象。
func NewUserInterestProfile(userID int64, model string, embedding []float64, embeddingJSON string, sampleCount int, lastEventID int64) (*UserInterestProfile, error) {
	if userID <= 0 {
		return nil, ErrInvalidUserID
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return nil, ErrEmptyEmbeddingModel
	}
	embeddingJSON = strings.TrimSpace(embeddingJSON)
	if !IsValidInterestVector(embedding) || embeddingJSON == "" {
		return nil, ErrEmptyInterestVector
	}
	if sampleCount < 0 {
		sampleCount = 0
	}
	if lastEventID < 0 {
		lastEventID = 0
	}
	return &UserInterestProfile{
		UserID:        userID,
		Model:         model,
		Dimension:     len(embedding),
		Embedding:     cloneFloat64Slice(embedding),
		EmbeddingJSON: embeddingJSON,
		SampleCount:   sampleCount,
		LastEventID:   lastEventID,
	}, nil

}

func cloneFloat64Slice(vector []float64) []float64 {
	cloned := make([]float64, len(vector))
	copy(cloned, vector)
	return cloned
}

// PositiveBehaviorEventTypes 返回参与用户兴趣向量聚合的正反馈事件类型列表。
func PositiveBehaviorEventTypes() []string {
	return []string{
		BehaviorEventPlay,
		BehaviorEventComplete,
	}
}
