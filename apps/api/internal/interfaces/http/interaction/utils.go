package interfacehttpinteraction

import (
	applicationinteraction "DreamReel/internal/application/interaction"
	domaininteraction "DreamReel/internal/domain/interaction"
	"DreamReel/internal/interfaces/http/middleware"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// userIDFromContext 从 JWT 中间件写入的上下文中读取当前登录用户 ID。
func userIDFromContext(c *gin.Context) (int64, bool) {
	// ContextUserIDKey 由 JWT 中间件写入，缺失时按未登录处理。
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

// parsePositiveInt64 统一解析路径参数和查询参数中的正整数 ID。
func parsePositiveInt64(raw string, fallback error) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, fallback
	}
	return value, nil
}

func writeInteractionError(c *gin.Context, err error) {
	// 统一错误映射让所有互动接口返回一致的 HTTP 状态码和 JSON 格式。
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domaininteraction.ErrVideoNotFound) || errors.Is(err, domaininteraction.ErrCommentNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "resource not found"})
		return
	}
	if errors.Is(err, domaininteraction.ErrCommentPermissionDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "comment permission denied"})
		return
	}
	// 返回 500 前记录错误日志，保留 root cause 便于排障。
	zap.L().Error("interaction request failed",
		zap.String("path", c.Request.URL.Path),
		zap.String("method", c.Request.Method),
		zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domaininteraction.ErrInvalidUserID) ||
		errors.Is(err, domaininteraction.ErrInvalidVideoID) ||
		errors.Is(err, domaininteraction.ErrInvalidCommentID) ||
		errors.Is(err, domaininteraction.ErrInvalidActionType) ||
		errors.Is(err, domaininteraction.ErrInvalidLimit) ||
		errors.Is(err, domaininteraction.ErrInvalidCursor) ||
		errors.Is(err, domaininteraction.ErrEmptyCommentContent) ||
		errors.Is(err, domaininteraction.ErrCommentContentTooLong) ||
		errors.Is(err, domaininteraction.ErrIdempotencyKeyTooLong)
}

func actionResponseFromResult(result *applicationinteraction.ActionResult) actionResponse {
	return actionResponse{
		VideoID:       result.VideoID,
		ActionType:    result.ActionType,
		Active:        result.Active,
		LikeCount:     result.LikeCount,
		FavoriteCount: result.FavoriteCount,
	}
}

func commentResponseFromResult(result *applicationinteraction.CreateCommentResult) commentResponse {
	response := commentResponse{
		ID:            result.Comment.ID,
		VideoID:       result.Comment.VideoID,
		UserID:        result.Comment.UserID,
		UserNickname:  result.Comment.UserNickname,
		UserAvatarURL: result.Comment.UserAvatarURL,
		Content:       result.Comment.Content,
		CreatedAt:     result.Comment.CreatedAt,
	}
	response.CommentCount = result.CommentCount
	return response
}

func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domaininteraction.ErrInvalidLimit
	}
	return limit, nil
}

func commentListResponseFromResult(result *applicationinteraction.CommentListResult) commentListResponse {
	items := make([]commentResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, commentResponseFromDomain(item))
	}
	return commentListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

func commentResponseFromDomain(comment *domaininteraction.Comment) commentResponse {
	return commentResponse{
		ID:            comment.ID,
		VideoID:       comment.VideoID,
		UserID:        comment.UserID,
		UserNickname:  comment.UserNickname,
		UserAvatarURL: comment.UserAvatarURL,
		Content:       comment.Content,
		CreatedAt:     comment.CreatedAt,
	}
}
