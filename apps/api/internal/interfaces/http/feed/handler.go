package interfacehttpfeed

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	applicationfeed "DreamReel/internal/application/feed"
	domainfeed "DreamReel/internal/domain/feed"
	httputils "DreamReel/internal/interfaces/http/utils"

	"github.com/gin-gonic/gin"
)

type Handler struct {
	service *applicationfeed.Service
}

// New 注入 Feed 应用服务。
func New(service *applicationfeed.Service) *Handler {
	return &Handler{service: service}
}

// ListFeedItems 列出指定 scene 的 Feed 列表，cursor 和 limit 来自 query 参数。
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

	c.JSON(http.StatusOK, TurnApplicationResultIntoResponse(result))
}

// Query 通过请求体接收复杂 Feed 查询参数，适合推荐上下文逐步扩展。
func (h *Handler) Query(c *gin.Context) {
	var req feedQueryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	limit, err := parseBodyLimit(req.Limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	viewerID, _ := httputils.GetUserID(c)
	result, err := h.service.GetFeed(c.Request.Context(), applicationfeed.FeedRequest{
		Scene:         domainfeed.Scene(req.Scene),
		Cursor:        req.Cursor,
		Limit:         limit,
		ViewerID:      viewerID,
		ClientContext: req.ClientContext,
	})
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, TurnApplicationResultIntoResponse(result))
}

// Refresh 从第一页重新读取 Feed，适合下拉刷新语义。
func (h *Handler) Refresh(c *gin.Context) {
	limit, err := parseLimit(c.Query("limit"))
	if err != nil {
		writeFeedError(c, err)
		return
	}

	result, err := h.service.RefreshFeed(c.Request.Context(), limit)
	if err != nil {
		writeFeedError(c, err)
		return
	}

	c.JSON(http.StatusOK, TurnApplicationResultIntoResponse(result))
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

// parseBodyLimit 校验 JSON 请求体中的 limit，空值交给应用服务使用默认页大小。
func parseBodyLimit(value *int) (int, error) {
	if value == nil {
		return 0, nil
	}
	if *value <= 0 {
		return 0, domainfeed.ErrInvalidLimit
	}
	return *value, nil
}
