package router

import (
	"database/sql"
	"net/http"

	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/metrics"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// Register 注册路由
func Register(g *gin.Engine, cfg *config.Config, db *sql.DB, rdb *redis.Client) error {
	g.GET("/health", HealthCheck(db, rdb))

	g.GET("/metrics", gin.WrapH(metrics.Handler()))

	return nil
}

func HealthCheck(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if err := db.PingContext(ctx); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"db":     "down",
			})
			return
		}
		if err := rdb.Ping(ctx).Err(); err != nil {
			c.JSON(http.StatusServiceUnavailable, gin.H{
				"status": "unhealthy",
				"redis":  "down",
			})
			return
		}
		c.JSON(http.StatusOK, gin.H{"status": "The server is running normally."})
	}
}
