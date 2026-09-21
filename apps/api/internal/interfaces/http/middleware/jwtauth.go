package middleware

import (
	jwt "DreamReel/internal/infra/jwt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const ContextUserIDKey = "userID"
const ContextRoleKey = "role"

// 鉴权中间件
func NewJWTAuth(jwtManager *jwt.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := strings.TrimSpace(c.GetHeader("Authorization"))
		if header == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "authorization header is required",
			})
			return
		}

		parts := strings.SplitN(header, " ", 2)
		if len(parts) != 2 {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authorization format must be Bearer <token>",
			})
			return
		}

		if !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "Authorization format must be Bearer <token>",
			})
			return
		}

		token := strings.TrimSpace(parts[1])
		claims, err := jwtManager.ParseAndValidateToken(token, jwt.TokenTypeAccess)

		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"message": "invalid access token",
			})
			return
		}

		c.Set(ContextUserIDKey, claims.UserID)
		c.Set(ContextRoleKey, claims.Role)
		c.Next()
	}

}
