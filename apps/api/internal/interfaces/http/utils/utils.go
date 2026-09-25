package httputils

import (
	"DreamReel/internal/interfaces/http/middleware"

	"github.com/gin-gonic/gin"
)

// GetuserID 从 JWT 中间件写入的上下文读取登录用户 ID。
func GetUserID(c *gin.Context) (int64, bool) {
	value, exists := c.Get(middleware.ContextUserIDKey)
	if !exists {
		return 0, false
	}
	userID, ok := value.(int64)
	return userID, ok && userID > 0
}
