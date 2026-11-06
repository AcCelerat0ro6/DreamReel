package interfacehttpvideo

import (
	applicationvideo "DreamReel/internal/application/video"
	"errors"
	"github.com/gin-gonic/gin"
	"net/http"
)

type RankingHandler struct {
	service *applicationvideo.RankingService
}

func NewRanking(service *applicationvideo.RankingService) *RankingHandler {
	return &RankingHandler{service: service}
}

type modelRankingResponse struct {
	Rank          int    `json:"rank"`
	ModelName     string `json:"model_name"`
	VideoCount    int64  `json:"video_count"`
	LikeCount     int64  `json:"like_count"`
	CommentCount  int64  `json:"comment_count"`
	FavoriteCount int64  `json:"favorite_count"`
	HotScore      int64  `json:"hot_score"`
}

func (h *RankingHandler) List(c *gin.Context) {
	limit, offset, err := parsePagination(c)
	if err != nil {
		writeVideoError(c, err)
		return
	}
	result, err := h.service.List(c.Request.Context(), c.Query("sort"), limit, offset)
	if errors.Is(err, applicationvideo.ErrInvalidRankingQuery) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to load model rankings"})
		return
	}
	items := make([]modelRankingResponse, 0, len(result.Items))
	for i, item := range result.Items {
		items = append(items, modelRankingResponse{
			Rank: result.Offset + i + 1, ModelName: item.ModelName, VideoCount: item.VideoCount,
			LikeCount: item.LikeCount, CommentCount: item.CommentCount,
			FavoriteCount: item.FavoriteCount, HotScore: item.HotScore,
		})
	}
	c.JSON(http.StatusOK, gin.H{"items": items, "sort": result.Sort, "limit": result.Limit, "offset": result.Offset, "has_more": result.HasMore})
}
