package applicationrecommendation

import (
	domainembedding "DreamReel/internal/domain/embedding"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	"context"
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
	// 进行同作者打散策略,避免同一作者的视频连续出现
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

// rankCandidates对粗粒度召回的候选进行细粒度重排
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
	// 1. 加载粗召回的候选视频对应的向量
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
	// 2. 计算候选视频的 freshnessScore 和 rankScore，并计算总得分
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
	// 将候选视频按照得分排序并返回最终结果
	sortCandidates(ranked)
	return ranked, nil
}
