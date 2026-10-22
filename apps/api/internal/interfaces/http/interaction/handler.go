package interfacehttpinteraction

import (
	applicationinteraction "DreamReel/internal/application/interaction"
	domaininteraction "DreamReel/internal/domain/interaction"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationinteraction.Service
}

func New(service *applicationinteraction.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Like(c *gin.Context) {
	h.setLike(c, true)
}

func (h *Handler) Unlike(c *gin.Context) {
	h.setLike(c, false)
}

func (h *Handler) setLike(c *gin.Context, active bool) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	videoID, err := parsePositiveInt64(c.Param("videoId"), domaininteraction.ErrInvalidVideoID)
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	var result *applicationinteraction.ActionResult
	if active {
		result, err = h.service.Like(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
	} else {
		result, err = h.service.Unlike(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
	}
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	c.JSON(http.StatusOK, actionResponseFromResult(result))
}

func (h *Handler) Favorite(c *gin.Context) {
	// 收藏和点赞共享同一套状态模型，只是 action_type 不同。
	h.setFavorite(c, true)
}

func (h *Handler) Unfavorite(c *gin.Context) {
	h.setFavorite(c, false)
}

func (h *Handler) setFavorite(c *gin.Context, active bool) {
	// 收藏和取消收藏共用参数解析逻辑，active 决定最终状态。
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	videoID, err := parsePositiveInt64(c.Param("videoId"), domaininteraction.ErrInvalidVideoID)
	if err != nil {
		writeInteractionError(c, err)
		return
	}

	var result *applicationinteraction.ActionResult
	if active {
		result, err = h.service.Favorite(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
	} else {
		result, err = h.service.Unfavorite(c.Request.Context(), userID, videoID, c.GetHeader("Idempotency-Key"))
	}
	if err != nil {
		writeInteractionError(c, err)
		return
	}
	c.JSON(http.StatusOK, actionResponseFromResult(result))
}
