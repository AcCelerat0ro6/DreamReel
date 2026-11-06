package applicationrecommendation

import (
	domainembedding "DreamReel/internal/domain/embedding"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	"context"
	"encoding/json"
	"errors"
)

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
func (s *Service) rankCandidates(ctx context.Context, session SessionState, pool []*domainrecommendation.Candidate) ([]*domainrecommendation.Candidate, error) {
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
	ranked := make([]*domainrecommendation.Candidate, 0, len(pool))
	for _, candidate := range pool {
		if candidate == nil {
			continue
		}
		value := *candidate
		value.FreshnessScore = freshnessScore(session.RankedAt, value.PublishedAt)
		value.Similarity = 0
		if len(session.UserVector) > 0 {
			if vector := vectors[value.VideoID]; len(vector) > 0 {
				similarity, err := domainembedding.CosineSimilarity(session.UserVector, vector)
				if err == nil {
					value.Similarity = similarity
				}
			}
		}
		value.RankScore = rankScore(value.Similarity, value.HotScore, value.FreshnessScore, len(session.UserVector) > 0)
		value.Reason = recommendationReason(len(session.UserVector) > 0, value.Similarity, value.HotScore)
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
