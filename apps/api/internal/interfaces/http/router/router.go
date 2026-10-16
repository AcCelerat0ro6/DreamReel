package router

import (
	"context"
	"database/sql"
	"net/http"

	applicationaccount "DreamReel/internal/application/account"
	applicationvideo "DreamReel/internal/application/video"
	domainfeed "DreamReel/internal/domain/feed"
	cache "DreamReel/internal/infra/cache"
	"DreamReel/internal/infra/config"
	jwt "DreamReel/internal/infra/jwt"
	"DreamReel/internal/infra/metrics"
	infraaccount "DreamReel/internal/infra/persistence/account"
	infrafeed "DreamReel/internal/infra/persistence/feed"
	"DreamReel/internal/infra/persistence/migration"
	infravideo "DreamReel/internal/infra/persistence/video"
	interfaceshttpaccount "DreamReel/internal/interfaces/http/account"
	interfaceshttpfeed "DreamReel/internal/interfaces/http/feed"
	"DreamReel/internal/interfaces/http/middleware"
	interfacehttprelation "DreamReel/internal/interfaces/http/relation"
	interfacehttpvideo "DreamReel/internal/interfaces/http/video"

	applicationrecommendation "DreamReel/internal/application/recommendation"
	infrarecommendation "DreamReel/internal/infra/persistence/recommendation"

	applicationfeed "DreamReel/internal/application/feed"

	applicationrelation "DreamReel/internal/application/relation"
	infrarelation "DreamReel/internal/infra/persistence/relation"

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

	// 1.3 Redis 初始化
	var feedCache *cache.FeedCache
	if cfg.Redis.Addr != "" {
		redisClient := cache.NewRedisClient(cfg.Redis)
		feedCache = cache.NewFeedCache(redisClient)
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
	// 4. Recommendation 域：候选召回、曝光决策、内部接口
	//    对外暴露 recommendationService 供 Feed 域作为 Recommender。
	// ============================================================

	recommendationRepo := infrarecommendation.New(gormDB)
	recommendationService := applicationrecommendation.New(recommendationRepo)
	// recommendationHandler := interfaceshttprecommendation.New(recommendationService)
	// ============================================================
	// 5. Feed 域：时间线、关注流、热榜、推荐流
	//    Option 集中装配：
	//      - Recommender        （必需，来自 Recommendation 域）
	//      - FeedCache          （Redis 可用时）
	// ============================================================
	feedRepo := infrafeed.New(gormDB)
	feedOptions := []applicationfeed.Option{
		applicationfeed.WithRecommender(recommendationService),
	}
	if feedCache != nil {
		feedOptions = append(feedOptions, applicationfeed.WithFeedCache(feedCache))
	}
	feedService := applicationfeed.New(feedRepo, feedOptions...)
	feedHandler := interfaceshttpfeed.New(feedService)

	// ============================================================
	// 6. Relation 域：关注、粉丝、关注流回填
	//    Option 集中装配：
	//      - MessageWriter          （必需，来自 Message 域）
	//      - FollowFeedBackfiller   （Redis 可用时，复用 feedRepo）
	// ============================================================
	relationRepo := infrarelation.New(gormDB)
	/*
		relationOptions := []applicationrelation.Option{
			applicationrelation.WithMessageWriter(messageWriter),
		}
	*/
	var relationOptions []applicationrelation.Option
	if feedCache != nil {
		relationOptions = append(relationOptions,
			applicationrelation.WithFollowFeedBackfiller(NewFollowFeedBackfiller(feedRepo, feedCache)),
		)
	}
	relationService := applicationrelation.New(relationRepo, relationOptions...)
	relationHandler := interfacehttprelation.New(relationService)

	// ============================================================
	// 7. 中间件
	// ============================================================
	authMiddleware := middleware.NewJWTAuth(jwtManager)
	optionalAuthMiddleware := middleware.NewOptionalJWTAuth(jwtManager)

	// ============================================================
	// 8. 路由注册：健康检查、指标、静态资源、公共 API、内部 API
	// ============================================================

	api := g.Group("/api")

	// 会话资源用于登录态：创建会话表示登录，删除当前会话表示登出。
	session := api.Group("/sessions")
	session.POST("", accountHandler.Login)
	session.DELETE("/cur", authMiddleware, accountHandler.Logout)

	// 用户资源承载注册、当前用户资料和用户作品列表。
	users := api.Group("/users")
	users.POST("", accountHandler.Register)
	users.PUT("/me/following/:targetUserId", authMiddleware, relationHandler.Follow)
	users.DELETE("/me/following/:targetUserId", authMiddleware, relationHandler.Unfollow)

	// 视频资源
	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	videos.GET("/:videoId", videoHandler.Get)

	// Feed流资源 用户通过游标控制获取视频列表
	api.GET("/feed-items", optionalAuthMiddleware, feedHandler.ListFeedItems)

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

func NewFollowFeedBackfiller(feedRepo interface {
	CountFollowers(ctx context.Context, authorID int64) (int, error)
	ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error)
}, feedCache interface {
	AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
}) *FollowFeedBackfiller {
	return &FollowFeedBackfiller{feedRepo: feedRepo, feedCache: feedCache}
}

func (b *FollowFeedBackfiller) CountFollowers(ctx context.Context, authorID int64) (int, error) {
	return b.feedRepo.CountFollowers(ctx, authorID)
}

func (b *FollowFeedBackfiller) ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error) {
	return b.feedRepo.ListAuthorRecentVideos(ctx, authorID, limit)
}

func (b *FollowFeedBackfiller) AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error {
	return b.feedCache.AddInboxItems(ctx, authorID, userIDs, item, maxLen)
}

type FollowFeedBackfiller struct {
	feedRepo interface {
		CountFollowers(ctx context.Context, authorID int64) (int, error)
		ListAuthorRecentVideos(ctx context.Context, authorID int64, limit int) ([]*domainfeed.FeedPageItem, error)
	}
	feedCache interface {
		AddInboxItems(ctx context.Context, authorID int64, userIDs []int64, item *domainfeed.FeedPageItem, maxLen int64) error
	}
}
