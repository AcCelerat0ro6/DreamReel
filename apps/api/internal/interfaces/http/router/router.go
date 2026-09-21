package router

import (
	"database/sql"
	"net/http"

	applicationaccount "DreamReel/internal/application/account"
	applicationvideo "DreamReel/internal/application/video"
	"DreamReel/internal/infra/config"
	jwt "DreamReel/internal/infra/jwt"
	"DreamReel/internal/infra/metrics"
	infraaccount "DreamReel/internal/infra/persistence/account"
	"DreamReel/internal/infra/persistence/migration"
	infravideo "DreamReel/internal/infra/persistence/video"
	interfaceshttpaccount "DreamReel/internal/interfaces/http/account"
	"DreamReel/internal/interfaces/http/middleware"
	interfacehttpvideo "DreamReel/internal/interfaces/http/video"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Register 注册路由
func Register(g *gin.Engine, cfg *config.Config, db *sql.DB, rdb *redis.Client) error {
	// ============================================================
	// 1. 基础设施：数据库、JWT、Redis、RabbitMQ
	// ============================================================

	// 1.1 GORM连接初始化,复用连接
	gormDB, err := gorm.Open(gormmysql.New(gormmysql.Config{
		Conn: db,
	}), &gorm.Config{})
	if err != nil {
		return err
	}

	// 1.2  创建表结构
	if err := migration.AutoMigrate(gormDB); err != nil {
		return err
	}

	// ============================================================
	// 2. Account 域：注册、登录、资料、JWT 签发
	// ============================================================
	jwtManager, err := jwt.NewManager(cfg.JWT.Secret, cfg.JWT.AccessTTL)
	if err != nil {
		return err
	}
	accountRepo := infraaccount.New(gormDB)
	accountService := applicationaccount.New(accountRepo, jwtManager)
	accountHandler := interfaceshttpaccount.New(accountService)

	// ============================================================
	// 3. Video 域：视频发布、查询、软删除
	// ============================================================
	/*
		后续RabbitMQ初始化:
		videoOptions := []applicationvideo.Option{}
		if rabbitMQ != nil {
			videoOptions = append(videoOptions, applicationvideo.WithPublishedEventPublisher(rabbitMQ))
		}
	*/
	videoRepo := infravideo.New(gormDB)
	videoService := applicationvideo.New(videoRepo)
	videoHandler := interfacehttpvideo.New(videoService)
	// ============================================================
	// 3. 中间件
	// ============================================================
	authMiddleware := middleware.NewJWTAuth(jwtManager)

	// ============================================================
	// 4. 路由注册：健康检查、指标、静态资源、公共 API、内部 API
	// ============================================================

	api := g.Group("/api")

	// 会话资源用于登录态：创建会话表示登录，删除当前会话表示登出。
	session := api.Group("/sessions")
	session.POST("", accountHandler.Login)
	session.DELETE("/cur", authMiddleware, accountHandler.Logout)

	// 用户资源承载注册、当前用户资料和用户作品列表。
	users := api.Group("/users")
	users.POST("", accountHandler.Register)

	// 视频资源
	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)

	// 检查路由提供后端服务基本状况检查
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
