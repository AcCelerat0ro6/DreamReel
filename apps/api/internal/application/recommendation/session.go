package applicationrecommendation

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	domainrecommendation "DreamReel/internal/domain/recommendation"
)

const (
	sessionBatchSize    = 200
	sessionLowWatermark = 30
	SessionTTL          = 15 * time.Minute
)

// SessionState 保存会话的评分依据和队列进度，普通翻页直接复用已排好的候选。
type SessionState struct {
	UserID       int64
	Scene        string
	RankedAt     time.Time // 推荐时间
	UserVector   []float64 // 用户向量
	Version      int       // 版本 用于乐观锁
	Count        int       //
	LastAuthorID int64
	Exhausted    bool
}

// SessionStore 按位置读取队列；补池仅在预期版本匹配时提交，避免并发请求重复追加。
type SessionStore interface {
	Read(ctx context.Context, id string, offset, count int) (SessionState, []*domainrecommendation.Candidate, error)
	EnqueuedIDs(ctx context.Context, id string) ([]int64, error)
	Append(ctx context.Context, id string, expectedVersion int, next SessionState, items []*domainrecommendation.Candidate) (bool, error)
}

// WithSessionStore 为推荐服务注入共享会话存储。
func WithSessionStore(store SessionStore) Option {
	return func(s *Service) { s.sessions = store }
}

// Recommend 创建或续读当前用户的推荐会话。
func (s *Service) Recommend(ctx context.Context, input CandidateRequest) (*CandidateResult, error) {
	cursor, err := parseCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	req, err := domainrecommendation.NewCandidateRequest(input.UserID, input.Scene, input.RequestID, cursor, normalizeLimit(input.Limit))
	if err != nil {
		return nil, err
	}

	if cursor == nil {
		cursor, err = s.startSession(ctx, req)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrLoadRecommendationFailed, err)
		}
	}
	result, err := s.readSessionPage(ctx, req, cursor)
	if err != nil && !errors.Is(err, domainrecommendation.ErrInvalidCursor) && !errors.Is(err, domainrecommendation.ErrCursorExpired) {
		return nil, fmt.Errorf("%w: %w", ErrLoadRecommendationFailed, err)
	}
	return result, err
}

func (s *Service) startSession(ctx context.Context, req *domainrecommendation.CandidateRequest) (*domainrecommendation.Cursor, error) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}
	id := hex.EncodeToString(nonce[:])
	vector, found, err := s.repo.LoadUserInterestVector(ctx, req.UserID)
	if err != nil {
		return nil, err
	}
	if !found {
		vector = nil
	}
	state := SessionState{UserID: req.UserID, Scene: req.Scene, RankedAt: s.now(), UserVector: vector}
	created, err := s.refill(ctx, id, state)
	if err != nil {
		return nil, err
	}
	if !created {
		return nil, errors.New("recommendation session already exists")
	}
	return &domainrecommendation.Cursor{SessionID: id}, nil
}

// refill 进行补充候选池操作， 从数据库中取出最近窗口时期内未被曝光的候选视频
// 当会话初始化（首次进入）或队列中剩余未看视频低于阈值（小于 30 条）时，refill 会被触发，从数据库中预取下一批（200 条）候选视频，完成精排打分、作者打散，并追加到会话队列中。
func (s *Service) refill(ctx context.Context, id string, state SessionState) (bool, error) {
	var excluded []int64
	if state.Count > 0 {
		var err error
		// 获取指定推荐会话Session中，之前所有批次已经进入过队列的全部视频 ID 列表
		excluded, err = s.sessions.EnqueuedIDs(ctx, id)
		if err != nil {
			return false, err
		}
		if len(excluded) < state.Count {
			return false, domainrecommendation.ErrCursorExpired
		}
	}

	pool, err := s.repo.ListCandidatePool(ctx, state.UserID, excluded, sessionBatchSize+1)
	if err != nil {
		return false, err
	}
	// 在数据库操作时，我们多拉取了一条（sessionBatchSize + 1），用于判断是否还有更多候选视频
	// 如果此时实际取出的视频数量不等于 sessionBatchSize + 1，则说明已经没有更多候选视频
	state.Exhausted = len(pool) <= sessionBatchSize
	if !state.Exhausted {
		pool = pool[:sessionBatchSize]
	}
	ranked, err := s.rankCandidates(ctx, state, pool)
	if err != nil {
		return false, err
	}
	ranked = interleaveByAuthor(ranked, state.LastAuthorID)
	if len(ranked) > 0 {
		state.LastAuthorID = ranked[len(ranked)-1].AuthorID
	}
	previous := state.Version
	state.Version++
	state.Count += len(ranked)
	return s.sessions.Append(ctx, id, previous, state, ranked)
}

func (s *Service) readSessionPage(ctx context.Context, req *domainrecommendation.CandidateRequest, cursor *domainrecommendation.Cursor) (*CandidateResult, error) {
	result := &CandidateResult{UserID: req.UserID, Scene: req.Scene, RequestID: req.RequestID, Candidates: make([]*domainrecommendation.Candidate, 0, req.Limit)}
	pos := cursor.Offset
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		needed := req.Limit - len(result.Candidates)
		state, batch, err := s.sessions.Read(ctx, cursor.SessionID, pos, req.Limit+1)
		if err != nil {
			return nil, err
		}
		if state.UserID != req.UserID || state.Scene != req.Scene || pos > state.Count {
			return nil, domainrecommendation.ErrInvalidCursor
		}
		if !state.Exhausted && state.Count-pos-needed < sessionLowWatermark {
			if _, err = s.refill(ctx, cursor.SessionID, state); err != nil {
				return nil, err
			}
			// 包括并发补池的情况，重新读取已提交的版本和队列。
			continue
		}
		if len(batch) == 0 && pos < state.Count {
			return nil, domainrecommendation.ErrCursorExpired
		}
		ids := make([]int64, 0, len(batch))
		for _, item := range batch {
			ids = append(ids, item.VideoID)
		}
		if len(ids) > 0 {
			visible, err := s.repo.PublishedVideoIDs(ctx, ids)
			if err != nil {
				return nil, err
			}
			for _, item := range batch {
				if visible[item.VideoID] {
					if len(result.Candidates) == req.Limit {
						// 多读到的第一条可见视频留给下一页，游标按实际扫描位置推进。
						result.HasMore = true
						result.NextCursor = encodeCursor(&domainrecommendation.Cursor{SessionID: cursor.SessionID, Offset: pos})
						return result, nil
					}
					result.Candidates = append(result.Candidates, item)
				}
				pos++
			}
		}
		if pos == state.Count && state.Exhausted {
			return result, nil
		}
	}
}
