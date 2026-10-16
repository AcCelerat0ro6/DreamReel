package interfacehttprelation

import (
	applicationrelation "DreamReel/internal/application/relation"
	domainrelation "DreamReel/internal/domain/relation"
	"DreamReel/internal/interfaces/http/middleware"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

func parsePositiveInt64(raw string, fallback error) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, fallback
	}
	return value, nil
}

func writeRelationError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainrelation.ErrTargetUserNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "target user not found"})
		return
	}
	if errors.Is(err, domainrelation.ErrIdempotencyKeyConflict) {
		c.JSON(http.StatusConflict, gin.H{"error": "idempotency key reused for a different follow state"})
		return
	}
	zap.L().Error("relation handler internal error",
		zap.String("method", c.Request.Method),
		zap.String("path", c.FullPath()),
		zap.Error(err),
	)
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domainrelation.ErrInvalidUserID) ||
		errors.Is(err, domainrelation.ErrInvalidTargetUserID) ||
		errors.Is(err, domainrelation.ErrFollowSelfForbidden) ||
		errors.Is(err, domainrelation.ErrInvalidLimit) ||
		errors.Is(err, domainrelation.ErrInvalidCursor) ||
		errors.Is(err, domainrelation.ErrIdempotencyKeyTooLong)
}

func followResponseFromResult(result *applicationrelation.FollowResult) followResponse {
	return followResponse{
		UserID:         result.UserID,
		TargetUserID:   result.TargetUserID,
		Status:         result.Status,
		Following:      result.Following,
		FollowingCount: result.FollowingCount,
		FollowerCount:  result.FollowerCount,
	}
}

func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domainrelation.ErrInvalidLimit
	}
	return limit, nil
}

func relationListResponseFromResult(result *applicationrelation.ListResult) relationListResponse {
	items := make([]relationUserResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, relationUserResponse{
			UserID:     item.UserID,
			Nickname:   item.Nickname,
			AvatarURL:  item.AvatarURL,
			Bio:        item.Bio,
			FollowedAt: item.FollowedAt,
		})
	}
	return relationListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}
