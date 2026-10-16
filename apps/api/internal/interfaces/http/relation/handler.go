package interfacehttprelation

import (
	applicationrelation "DreamReel/internal/application/relation"
	domainrelation "DreamReel/internal/domain/relation"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationrelation.Service
}

// New 创建关系 HTTP Handler。
func New(service *applicationrelation.Service) *Handler {
	return &Handler{service: service}
}

// Follow 处理关注用户接口。
func (h *Handler) Follow(c *gin.Context) {
	h.setFollow(c, true)
}

// Unfollow 处理取消关注用户接口。
func (h *Handler) Unfollow(c *gin.Context) {
	h.setFollow(c, false)
}

func (h *Handler) setFollow(c *gin.Context, active bool) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	targetUserID, err := parsePositiveInt64(c.Param("targetUserId"), domainrelation.ErrInvalidTargetUserID)
	if err != nil {
		writeRelationError(c, err)
		return
	}
	var result *applicationrelation.FollowResult
	if active {
		result, err = h.service.Follow(c.Request.Context(), userID, targetUserID, c.GetHeader("Idempotency-Key"))
	} else {
		result, err = h.service.Unfollow(c.Request.Context(), userID, targetUserID, c.GetHeader("Idempotency-Key"))
	}
	if err != nil {
		writeRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, followResponseFromResult(result))

}

// ListFollowing 查询当前用户关注列表。
func (h *Handler) ListFollowing(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeRelationError(c, err)
		return
	}

	result, err := h.service.ListFollowing(c.Request.Context(), userID, c.Query("cursor"), limit)

	if err != nil {
		writeRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, relationListResponseFromResult(result))
}

// ListFollowers 查询当前用户粉丝列表。
func (h *Handler) ListFollowers(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeRelationError(c, err)
		return
	}

	result, err := h.service.ListFollowers(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeRelationError(c, err)
		return
	}
	c.JSON(http.StatusOK, relationListResponseFromResult(result))
}
