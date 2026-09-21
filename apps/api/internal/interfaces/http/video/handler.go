package interfacehttpvideo

import (
	applicationvideo "DreamReel/internal/application/video"
	domainvideo "DreamReel/internal/domain/video"
	utils "DreamReel/internal/interfaces/http/utils"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	service *applicationvideo.Service
}

func New(service *applicationvideo.Service) *Handler {
	return &Handler{service: service}
}

func (h *Handler) Create(c *gin.Context) {
	// 1. 获取userID
	userID, ok := utils.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	// 2. 参数绑定
	var req CreateVideoRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request param"})
		return
	}

	result, err := h.service.CreatePublished(
		c.Request.Context(),
		userID,
		req.Title,
		req.Description,
		req.MediaURL,
		req.CoverURL,
		req.ModelName,
		req.ModelParams,
		req.AIStyleTag,
		req.OriginVideoID,
		c.GetHeader("Idempotency-Key"),
	)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	status := http.StatusCreated
	if !result.Created {
		// 幂等重放返回已有资源，使用 200 表示本次没有新建记录。
		status = http.StatusOK
	}
	c.JSON(status, videoCreateResponseFromDomain(result.Video))

}

// Delete 删除当前用户自己的视频，删除操作在领域层做作者权限校验。
func (h *Handler) Delete(c *gin.Context) {
	userID, ok := utils.GetUserID(c)
	if !ok {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid access token"})
		return
	}

	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}

	if err := h.service.Delete(c.Request.Context(), userID, videoID); err != nil {
		writeVideoError(c, err)
		return
	}

	c.Status(http.StatusNoContent)
}

// Get 查询公开视频
func (h *Handler) Get(c *gin.Context) {
	videoID, err := parsePositiveInt64(c.Param("videoId"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid video id"})
		return
	}
	video, err := h.service.Get(c.Request.Context(), videoID)
	if err != nil {
		writeVideoError(c, err)
		return
	}

	c.JSON(http.StatusOK, videoCreateResponseFromDomain(video))
}

func parsePositiveInt64(raw string) (int64, error) {
	value, err := strconv.ParseInt(strings.TrimSpace(raw), 10, 64)
	if err != nil || value <= 0 {
		return 0, domainvideo.ErrInvalidVideoID
	}
	return value, nil
}

func writeVideoError(c *gin.Context, err error) {
	if isBadRequestError(err) {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "video not found"})
		return
	}
	if errors.Is(err, domainvideo.ErrVideoPermissionDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": "video permission denied"})
		return
	}
	zap.L().Error("video handler internal error", zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
}

// isBadRequestError 判断哪些视频领域错误属于客户端请求问题。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainvideo.ErrInvalidVideoID) ||
		errors.Is(err, domainvideo.ErrInvalidAuthorID) ||
		errors.Is(err, domainvideo.ErrEmptyTitle) ||
		errors.Is(err, domainvideo.ErrTitleTooLong) ||
		errors.Is(err, domainvideo.ErrDescriptionTooLong) ||
		errors.Is(err, domainvideo.ErrEmptyMediaURL) ||
		errors.Is(err, domainvideo.ErrEmptyCoverURL) ||
		errors.Is(err, domainvideo.ErrIdempotencyKeyTooLong) ||
		errors.Is(err, domainvideo.ErrInvalidLimit) ||
		errors.Is(err, domainvideo.ErrInvalidOffset) ||
		errors.Is(err, domainvideo.ErrModelNameTooLong) ||
		errors.Is(err, domainvideo.ErrPromptTooLong) ||
		errors.Is(err, domainvideo.ErrModelParamsTooLong) ||
		errors.Is(err, domainvideo.ErrAIStyleTagTooLong)
}
