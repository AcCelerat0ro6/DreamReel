package applicationrecommendation

import (
	domainembedding "DreamReel/internal/domain/embedding"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	"context"
	"encoding/json"
	"errors"
)

func (s *Service) Recommend(ctx context.Context, input CandidateRequest) (*CandidateResult, error) {
	limit := normalizeLimit(input.Limit)
	cursor, err := parseCursor(input.Cursor)
	if err != nil {
		return nil, err
	}
	req, err := domainrecommendation.NewCandidateRequest(input.UserID, input.Scene, input.RequestID, cursor, limit)
	if err != nil {
		return nil, err
	}

	// 放大候选池，后续再进行重排筛选
	poolLimit := candidatePoolLimit(limit)

	// 粗粒度召回
	pool, err := s.repo.ListCandidatePool(ctx, req.UserID, poolLimit)
	if err != nil {
		return nil, ErrLoadRecommendationFailed
	}

	// 细粒度重排
	ranked, err := s.rankCandidates(ctx, req.UserID, pool)
	if err != nil {
		return nil, ErrLoadRecommendationFailed
	}

	// 过滤掉游标之前的内容
	ranked = filterByCursor(ranked, req.Cursor)

	hasMore := len(ranked) > limit
	if hasMore {
		ranked = ranked[:limit]
	}
	nextCursor := ""
	if len(ranked) > 0 {
		nextCursor = encodeCursor(&domainrecommendation.Cursor{
			RankScore:   ranked[len(ranked)-1].RankScore,
			PublishedAt: ranked[len(ranked)-1].PublishedAt,
			VideoID:     ranked[len(ranked)-1].VideoID,
		})
	}
	// 进行同作者打散策略，避免同一作者的视频连续出现
	ranked = interleaveByAuthor(ranked)

	return &CandidateResult{
		UserID:     req.UserID,
		Scene:      req.Scene,
		RequestID:  req.RequestID,
		Candidates: ranked,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}, nil
}

// DecideExposures 对一批候选视频执行近期曝光窗口去重判定。
func (s *Service) DecideExposures(ctx context.Context, input ExposureDecisionInput) (*ExposureDecisionResult, error) {
	req, err := domainrecommendation.NewExposureDecisionRequest(input.UserID, input.Scene, input.RequestID, input.VideoIDs)
	if err != nil {
		return nil, err
	}
	if len(req.VideoIDs) == 0 {
		return &ExposureDecisionResult{
			UserID:    req.UserID,
			Scene:     req.Scene,
			RequestID: req.RequestID,
			Decisions: []*domainrecommendation.ExposureDecision{},
		}, nil
	}

	exposures, err := s.repo.ListRecentExposures(ctx, req.UserID, req.VideoIDs, s.now().Add(-domainrecommendation.RecentExposureWindow))
	if err != nil {
		return nil, ErrLoadExposureDecisionsFailed
	}
	exposureByVideoID := make(map[int64]*domainrecommendation.Exposure, len(exposures))
	for _, exposure := range exposures {
		if exposure != nil {
			exposureByVideoID[exposure.VideoID] = exposure
		}
	}

	decisions := make([]*domainrecommendation.ExposureDecision, 0, len(req.VideoIDs))
	for _, videoID := range req.VideoIDs {
		if exposure := exposureByVideoID[videoID]; exposure != nil {
			decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
				videoID,
				false,
				domainrecommendation.ExposureDecisionReasonRecentlyExposed,
				&exposure.LastExposedAt,
			))
			continue
		}
		decisions = append(decisions, domainrecommendation.RestoreExposureDecision(
			videoID,
			true,
			domainrecommendation.ExposureDecisionReasonFresh,
			nil,
		))
	}
	return &ExposureDecisionResult{
		UserID:    req.UserID,
		Scene:     req.Scene,
		RequestID: req.RequestID,
		Decisions: decisions,
	}, nil
}

// SaveExposures 批量校验、去重并保存曝光记录。
func (s *Service) SaveExposures(ctx context.Context, inputs []ExposureInput) (*ExposureResult, error) {
	writes := make([]*domainrecommendation.ExposureWrite, 0, len(inputs))
	seen := map[int64]struct{}{}
	for _, input := range inputs {
		write, err := domainrecommendation.NewExposureWrite(input.UserID, input.VideoID, input.Scene, input.RequestID)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[write.VideoID]; exists {
			continue
		}
		seen[write.VideoID] = struct{}{}
		writes = append(writes, write)
	}
	if len(writes) == 0 {
		return &ExposureResult{Exposures: []*domainrecommendation.Exposure{}}, nil
	}

	exposures, err := s.repo.SaveExposures(ctx, writes)
	if err != nil {
		if errors.Is(err, domainrecommendation.ErrVideoNotFound) {
			return nil, err
		}
		return nil, ErrSaveRecommendationExposureFailed
	}
	return &ExposureResult{Exposures: exposures}, nil
}

// rankCandidates 对粗粒度召回的候选进行细粒度重排。
func (s *Service) rankCandidates(ctx context.Context, userID int64, pool []*domainrecommendation.Candidate) ([]*domainrecommendation.Candidate, error) {
	if len(pool) == 0 {
		return []*domainrecommendation.Candidate{}, nil
	}

	videoIDs := make([]int64, 0, len(pool))
	for _, candidate := range pool {
		if candidate != nil && candidate.VideoID > 0 {
			videoIDs = append(videoIDs, candidate.VideoID)
		}
	}
	vectors, err := s.repo.LoadVideoVectors(ctx, videoIDs)
	if err != nil {
		return nil, err
	}
	userVector, hasUserVector, err := s.repo.LoadUserInterestVector(ctx, userID)
	if err != nil {
		return nil, err
	}

	now := s.now()
	ranked := make([]*domainrecommendation.Candidate, 0, len(pool))
	for _, candidate := range pool {
		if candidate == nil {
			continue
		}
		value := *candidate
		value.FreshnessScore = freshnessScore(now, value.PublishedAt)
		value.Similarity = 0
		if hasUserVector {
			if vector := vectors[value.VideoID]; len(vector) > 0 {
				similarity, err := domainembedding.CosineSimilarity(userVector, vector)
				if err == nil {
					value.Similarity = similarity
				}
			}
		}
		value.RankScore = rankScore(value.Similarity, value.HotScore, value.FreshnessScore, hasUserVector)
		value.Reason = recommendationReason(hasUserVector, value.Similarity, value.HotScore)
		ranked = append(ranked, &value)
	}

	sortCandidates(ranked)
	return ranked, nil
}

// RefreshUserInterestProfile 根据观看行为事件聚合用户近期正反馈样本，并更新用户推荐兴趣向量。
func (s *Service) RefreshUserInterestProfile(ctx context.Context, input RefreshProfileInput) (*RefreshProfileResult, error) {
	if s == nil || s.repo == nil || input.UserID <= 0 {
		return &RefreshProfileResult{}, nil
	}
	if !domainrecommendation.IsProfileTriggerEvent(input.EventType, input.Completed) {
		return &RefreshProfileResult{}, nil
	}

	since := s.now().Add(-domainrecommendation.PositiveBehaviorWindow)
	samples, err := s.repo.ListRecentBehaviorSamples(
		ctx,
		input.UserID,
		domainembedding.HashNgramModel,
		since,
		domainrecommendation.MaxProfileSampleEvents,
	)
	if err != nil {
		return nil, ErrUpdateUserProfileFailed
	}

	vector, sampleCount := domainrecommendation.AggregateInterestVector(samples)
	if len(vector) == 0 || sampleCount == 0 {
		return &RefreshProfileResult{}, nil
	}

	content, err := json.Marshal(vector)
	if err != nil {
		return nil, ErrMarshalInterestVectorFailed
	}

	profile, err := domainrecommendation.NewUserInterestProfile(
		input.UserID,
		domainembedding.HashNgramModel,
		vector,
		string(content),
		sampleCount,
		input.ViewEventID,
	)
	if err != nil {
		return nil, err
	}
	if err := s.repo.SaveUserInterestProfile(ctx, profile); err != nil {
		return nil, ErrUpdateUserProfileFailed
	}

	return &RefreshProfileResult{
		Profile: profile,
		Updated: true,
	}, nil
}
