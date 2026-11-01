package interfacehttpmessage

import (
	applicationmessage "DreamReel/internal/application/message"
	domainmessage "DreamReel/internal/domain/message"
	"DreamReel/internal/interfaces/http/middleware"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

func userIDFromContext(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}

func parseLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	limit, err := strconv.Atoi(raw)
	if err != nil || limit <= 0 {
		return 0, domainmessage.ErrInvalidLimit
	}
	return limit, nil
}

func writeMessageError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func isBadRequestError(err error) bool {
	return errors.Is(err, domainmessage.ErrInvalidUserID) ||
		errors.Is(err, domainmessage.ErrInvalidMessageID) ||
		errors.Is(err, domainmessage.ErrInvalidLimit) ||
		errors.Is(err, domainmessage.ErrInvalidCursor) ||
		errors.Is(err, domainmessage.ErrInvalidMessageType) ||
		errors.Is(err, domainmessage.ErrEmptyTitle) ||
		errors.Is(err, domainmessage.ErrTitleTooLong) ||
		errors.Is(err, domainmessage.ErrEmptyContent) ||
		errors.Is(err, domainmessage.ErrContentTooLong) ||
		errors.Is(err, domainmessage.ErrEventIDTooLong) ||
		errors.Is(err, domainmessage.ErrIdempotencyKeyTooLong)
}

func listResponseFromResult(result *applicationmessage.ListResult) messageListResponse {
	items := make([]messageResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, responseFromDomain(item))
	}
	return messageListResponse{
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}

func responseFromDomain(message *domainmessage.Message) messageResponse {
	return messageResponse{
		ID:             message.ID,
		UserID:         message.UserID,
		Type:           message.Type,
		Title:          message.Title,
		Content:        message.Content,
		EventID:        message.EventID,
		ActorID:        message.ActorID,
		ActorNickname:  message.ActorNickname,
		ActorAvatarURL: message.ActorAvatarURL,
		IsRead:         message.IsRead,
		CreatedAt:      message.CreatedAt,
		ReadAt:         message.ReadAt,
	}
}
