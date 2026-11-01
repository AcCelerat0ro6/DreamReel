package interfacehttpmessage

import (
	applicationmessage "DreamReel/internal/application/message"
	"net/http"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationmessage.Service
}

func New(service *applicationmessage.Service) *Handler {
	return &Handler{service: service}
}

// list 查询当前用户的消息列表
func (h *Handler) List(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeMessageError(c, err)
		return
	}

	result, err := h.service.List(c.Request.Context(), userID, c.Query("cursor"), limit)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, listResponseFromResult(result))
}

// MarkRead 将当前登录用户的指定消息标记为已读。
func (h *Handler) MarkRead(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	var req markReadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	result, err := h.service.MarkRead(c.Request.Context(), userID, req.MessageIDs)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, markReadResponse{UpdatedCount: result.UpdatedCount})
}

// CountUnread 查询当前登录用户未读消息数。
func (h *Handler) CountUnread(c *gin.Context) {
	userID, ok := userIDFromContext(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}
	stat, err := h.service.CountUnread(c.Request.Context(), userID)
	if err != nil {
		writeMessageError(c, err)
		return
	}
	c.JSON(http.StatusOK, unreadStatResponse{UnreadCount: stat.UnreadCount})

}
