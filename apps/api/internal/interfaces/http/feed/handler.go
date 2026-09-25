package interfaceshttpfeed

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	domainfeed "DreamReel/internal/domain/feed"
	httputils "DreamReel/internal/interfaces/http/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
}

// ListFeedItems 列出指定 scene 的 Feed列表
func (h *Handler) ListFeedItems(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	userID, _ := httputils.GetUserID(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:    domainfeed.Scene(c.Query("scene")),
		Cursor:   c.Query("cursor"),
		Limit:    limit,
		ViewerID: userID,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, feedItemsResponseFromResult(result))

}

// isBadRequestError 判断 Feed 参数错误。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainfeed.ErrInvalidLimit) ||
		errors.Is(err, domainfeed.ErrInvalidCursor) ||
		errors.Is(err, domainfeed.ErrUnsupportedScene)
}

func writeFeedError(c *gin.Context, err error) {
	if errors.Is(err, domainfeed.ErrViewerRequired) {
		c.JSON(http.StatusUnauthorized, gin.H{"error": err.Error()})
		return
	}
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

func parseLimit(limit string) (int, error) {
	limit = strings.TrimSpace(limit)
	if limit == "" {
		return 0, nil
	}

	limited, err := strconv.Atoi(limit)
	if err != nil || limited <= 0 {
		return 0, domainfeed.ErrInvalidLimit
	}
	return limited, nil
}
