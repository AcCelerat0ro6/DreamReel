package router

import (
	"context"
	"database/sql"
	"net/http"

	applicationaccount "DreamReel/internal/application/account"
	applicationexposure "DreamReel/internal/application/exposure"
	applicationinteraction "DreamReel/internal/application/interaction"
	applicationvideo "DreamReel/internal/application/video"
	domainfeed "DreamReel/internal/domain/feed"
	cache "DreamReel/internal/infra/cache"
	"DreamReel/internal/infra/config"
	jwt "DreamReel/internal/infra/jwt"
	"DreamReel/internal/infra/metrics"
	"DreamReel/internal/infra/mq"
	infraaccount "DreamReel/internal/infra/persistence/account"
	infraexposure "DreamReel/internal/infra/persistence/exposure"
	infrafeed "DreamReel/internal/infra/persistence/feed"
	inframessage "DreamReel/internal/infra/persistence/message"
	infraplayback "DreamReel/internal/infra/persistence/playback"

	"DreamReel/internal/infra/persistence/migration"
	infravideo "DreamReel/internal/infra/persistence/video"
	interfaceshttpaccount "DreamReel/internal/interfaces/http/account"
	interfacehttpexposure "DreamReel/internal/interfaces/http/exposure"
	interfaceshttpfeed "DreamReel/internal/interfaces/http/feed"
	interfacehttpinteraction "DreamReel/internal/interfaces/http/interaction"
	"DreamReel/internal/interfaces/http/middleware"
	interfaceshttprecommendation "DreamReel/internal/interfaces/http/recommendation"
	interfacehttprelation "DreamReel/internal/interfaces/http/relation"
	interfacehttpvideo "DreamReel/internal/interfaces/http/video"

	applicationrecommendation "DreamReel/internal/application/recommendation"
	infrarecommendation "DreamReel/internal/infra/persistence/recommendation"

	applicationfeed "DreamReel/internal/application/feed"

	applicationrelation "DreamReel/internal/application/relation"
	infrarelation "DreamReel/internal/infra/persistence/relation"

	applicationmessage "DreamReel/internal/application/message"
	infrainteraction "DreamReel/internal/infra/persistence/interaction"
	interfaceshttpmessage "DreamReel/internal/interfaces/http/message"

	applicationplayback "DreamReel/internal/application/playback"
	interfacehttpplayback "DreamReel/internal/interfaces/http/playback"

	interfacehttpupload "DreamReel/internal/interfaces/http/upload"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// Register 注册路由
func Register(g *gin.Engine, cfg *config.Config, db *sql.DB, rdb *redis.Client, rabbitMQ *mq.RabbitMQ) error {
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

	// 1.3 Redis 初始化：优先复用入口层注入的 rdb；若未注入但配置了地址，则按配置创建客户端。
	var feedCache *cache.FeedCache
	if rdb != nil {
		feedCache = cache.NewFeedCache(rdb)
	} else if cfg.Redis.Addr != "" {
		redisClient := cache.NewRedisClient(cfg.Redis)
		feedCache = cache.NewFeedCache(redisClient)
	}

	// 1.4 RabbitMQ 客户端：由入口层注入并管理生命周期，供 Video/Exposure/Interaction 三个模块发布事件。

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
	videoOptions := []applicationvideo.Option{}
	if rabbitMQ != nil {
		videoOptions = append(videoOptions, applicationvideo.WithPublishedEventPublisher(rabbitMQ))
	}
	videoRepo := infravideo.New(gormDB)
	videoService := applicationvideo.New(videoRepo, videoOptions...)
	videoHandler := interfacehttpvideo.New(videoService)
	modelRankingHandler := interfacehttpvideo.NewRanking(applicationvideo.NewRanking(videoRepo))

	// ============================================================
	// 4. Message 域：站内消息中心
	//    对外暴露 messageWriter 供 Interaction/Relation 域消费。
	// ============================================================
	messageRepo := inframessage.New(gormDB)
	messageService := applicationmessage.New(messageRepo)
	messageHandler := interfaceshttpmessage.New(messageService)
	messageWriter := NewMessageWriter(messageService)

	// ============================================================
	// 5. Recommendation 域：候选召回、曝光决策、内部接口
	//    对外暴露 recommendationService 供 Feed 域作为 Recommender。
	// ============================================================
	recommendationRepo := infrarecommendation.New(gormDB)
	var recommendationOptions []applicationrecommendation.Option
	if rdb != nil {
		recommendationOptions = append(recommendationOptions, applicationrecommendation.WithSessionStore(cache.NewRecommendationSessionStore(rdb)))
	}
	recommendationService := applicationrecommendation.New(recommendationRepo, recommendationOptions...)
	recommendationHandler := interfaceshttprecommendation.New(recommendationService)

	// ============================================================
	// 6. Feed 域：时间线、关注流、热榜、推荐流
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
	// 7. Relation 域：关注、粉丝、关注流回填
	//    Option 集中装配：
	//      - MessageWriter          （必需，来自 Message 域）
	//      - FollowFeedBackfiller   （Redis 可用时，复用 feedRepo）
	// ============================================================
	relationRepo := infrarelation.New(gormDB)
	relationOptions := []applicationrelation.Option{
		applicationrelation.WithMessageWriter(messageWriter),
	}
	if feedCache != nil {
		relationOptions = append(relationOptions,
			applicationrelation.WithFollowFeedBackfiller(NewFollowFeedBackfiller(feedRepo, feedCache)),
		)
	}
	relationService := applicationrelation.New(relationRepo, relationOptions...)
	relationHandler := interfacehttprelation.New(relationService)

	// ============================================================
	// 8. Interaction 域：点赞、收藏、评论、热度分、异步落库
	//    Option 集中装配：
	//      - MessageWriter        （必需，来自 Message 域）
	//      - HotScoreRecorder     （Redis 可用时）
	//      - StatCache            （Redis 可用时）
	//      - AsyncActionPipeline  （Redis 和 RabbitMQ 同时可用时）
	// ============================================================
	interactionRepo := infrainteraction.New(gormDB)
	interactionOptions := []applicationinteraction.Option{
		applicationinteraction.WithMessageWriter(messageWriter),
	}
	if feedCache != nil {
		interactionOptions = append(interactionOptions,
			applicationinteraction.WithHotScoreRecorder(feedCache),
			applicationinteraction.WithStatCache(feedCache),
		)
		if rabbitMQ != nil {
			interactionOptions = append(interactionOptions,
				applicationinteraction.WithAsyncActionPipeline(feedCache, rabbitMQ),
			)
		}
	}
	interactionService := applicationinteraction.New(interactionRepo, interactionOptions...)
	interactionHandler := interfacehttpinteraction.New(interactionService)

	// ============================================================
	// 9. Exposure 域：曝光、播放、完播事件
	//    Option 集中装配：
	//      - ViewEventPublisher   （RabbitMQ 可用时）
	// ============================================================
	exposureRepo := infraexposure.New(gormDB)
	exposureOptions := []applicationexposure.Option{}
	if rabbitMQ != nil {
		exposureOptions = append(exposureOptions, applicationexposure.WithViewEventPublisher(rabbitMQ))
	}
	exposureService := applicationexposure.New(exposureRepo, exposureOptions...)
	exposureHandler := interfacehttpexposure.New(exposureService)

	// ============================================================
	// 10. Playback 域：播放配置、预加载建议、QoS 上报
	// ============================================================
	playbackRepo := infraplayback.New(gormDB)
	playbackService := applicationplayback.New(playbackRepo)
	playbackHandler := interfacehttpplayback.New(playbackService)

	// ============================================================
	// 11. Upload 域：文件上传（本地磁盘存储）
	// ============================================================
	uploadHandler := interfacehttpupload.New("./uploads")

	// ============================================================
	// 11. 中间件
	// ============================================================
	authMiddleware := middleware.NewJWTAuth(jwtManager)
	optionalAuthMiddleware := middleware.NewOptionalJWTAuth(jwtManager)
	internalAuthMiddleware := middleware.NewInternalTokenAuth(cfg.Internal.Token)

	// ============================================================
	// 12. 路由注册：健康检查、指标、静态资源、公共 API、内部 API
	// ============================================================

	api := g.Group("/api")
	api.GET("/model-rankings", modelRankingHandler.List)

	// 会话资源用于登录态：创建会话表示登录，删除当前会话表示登出。
	session := api.Group("/sessions")
	session.POST("", accountHandler.Login)
	session.DELETE("/cur", authMiddleware, accountHandler.Logout)
	session.DELETE("/current", authMiddleware, accountHandler.Logout)

	// 用户资源承载注册、当前用户资料、关系列表和用户作品列表。
	users := api.Group("/users")
	users.POST("", accountHandler.Register)
	users.GET("/me", authMiddleware, accountHandler.Me)
	users.PATCH("/me", authMiddleware, accountHandler.UpdateMe)
	users.GET("/me/videos", authMiddleware, videoHandler.ListMine)
	users.PUT("/me/following/:targetUserId", authMiddleware, relationHandler.Follow)
	users.DELETE("/me/following/:targetUserId", authMiddleware, relationHandler.Unfollow)
	users.GET("/me/following", authMiddleware, relationHandler.ListFollowing)
	users.GET("/me/followers", authMiddleware, relationHandler.ListFollowers)
	users.GET("/:userId", accountHandler.Get)
	users.GET("/:userId/videos", videoHandler.ListByAuthor)

	// 视频资源
	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	videos.GET("/:videoId", videoHandler.Get)
	videos.PUT("/:videoId/like", authMiddleware, interactionHandler.Like)
	videos.DELETE("/:videoId/like", authMiddleware, interactionHandler.Unlike)
	videos.PUT("/:videoId/favorite", authMiddleware, interactionHandler.Favorite)
	videos.DELETE("/:videoId/favorite", authMiddleware, interactionHandler.Unfavorite)
	videos.POST("/:videoId/comments", authMiddleware, interactionHandler.CreateComment)
	videos.GET("/:videoId/comments", interactionHandler.ListComments)

	// Feed流资源 用户通过游标控制获取视频列表
	api.GET("/feed-items", optionalAuthMiddleware, feedHandler.ListFeedItems)
	api.POST("/feed-queries", optionalAuthMiddleware, feedHandler.Query)
	api.DELETE("/comments/:commentId", authMiddleware, interactionHandler.DeleteComment)
	api.POST("/video-view-events", authMiddleware, exposureHandler.CreateViewEvent)

	// 消息资源
	api.GET("/messages", authMiddleware, messageHandler.List)
	api.PATCH("/messages", authMiddleware, messageHandler.MarkRead)
	api.GET("/message-stats/unread", authMiddleware, messageHandler.CountUnread)

	// 播放资源
	api.GET("/playback-config", authMiddleware, playbackHandler.GetConfig)
	api.GET("/preload-videos", authMiddleware, playbackHandler.ListPreloadVideos)
	api.POST("/playback-qos-reports", authMiddleware, playbackHandler.CreateQoSReport)

	// 视频上传资源
	// 静态文件路由让上传后的文件可以通过 /uploads/... 访问。
	g.Static("/uploads", "./uploads")
	uploads := api.Group("/uploads", authMiddleware)
	uploads.POST("", uploadHandler.Create)

	// 内部服务调用接口
	internal := g.Group("/internal")
	internal.POST("/recommendation-candidates", recommendationHandler.ListCandidates)
	internal.POST("/exposure-decisions", recommendationHandler.DecideExposures)
	internal.POST("/exposures", recommendationHandler.SaveExposures)
	internal.POST("/messages", internalAuthMiddleware, messageHandler.Create)
	internal.POST("/playback-qos-reports", internalAuthMiddleware, playbackHandler.CreateInternalQoSReport)

	// 检查路由提供后端服务基本状况检查
	g.GET("/health", HealthCheck(db, rdb))
	g.GET("/metrics", gin.WrapH(metrics.Handler()))

	return nil
}

func HealthCheck(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if db != nil {
			if err := db.PingContext(ctx); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "unhealthy",
					"db":     "down",
				})
				return
			}
		}
		if rdb != nil {
			if err := rdb.Ping(ctx).Err(); err != nil {
				c.JSON(http.StatusServiceUnavailable, gin.H{
					"status": "unhealthy",
					"redis":  "down",
				})
				return
			}
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

type MessageWriter struct {
	service *applicationmessage.Service
}

func NewMessageWriter(service *applicationmessage.Service) *MessageWriter {
	return &MessageWriter{service: service}
}

func (w *MessageWriter) CreateFromEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string) (any, error) {
	return w.service.CreateFromEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey)
}

func (w *MessageWriter) CreateFromActorEvent(ctx context.Context, userID int64, messageType string, title string, content string, eventID string, idempotencyKey string, actorID int64, actorNickname string, actorAvatarURL string) (any, error) {
	return w.service.CreateFromActorEvent(ctx, userID, messageType, title, content, eventID, idempotencyKey, actorID, actorNickname, actorAvatarURL)
}
