package test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	applicationaccount "DreamReel/internal/application/account"
	applicationfeed "DreamReel/internal/application/feed"
	applicationmessage "DreamReel/internal/application/message"
	applicationplayback "DreamReel/internal/application/playback"
	applicationrecommendation "DreamReel/internal/application/recommendation"
	applicationrelation "DreamReel/internal/application/relation"
	applicationvideo "DreamReel/internal/application/video"
	domainaccount "DreamReel/internal/domain/account"
	domainfeed "DreamReel/internal/domain/feed"
	domainmessage "DreamReel/internal/domain/message"
	domainplayback "DreamReel/internal/domain/playback"
	domainrecommendation "DreamReel/internal/domain/recommendation"
	domainrelation "DreamReel/internal/domain/relation"
	domainvideo "DreamReel/internal/domain/video"
	"DreamReel/internal/infra/jwt"
	interfaceshttpaccount "DreamReel/internal/interfaces/http/account"
	interfaceshttpfeed "DreamReel/internal/interfaces/http/feed"
	interfaceshttpmessage "DreamReel/internal/interfaces/http/message"
	"DreamReel/internal/interfaces/http/middleware"
	interfacehttpplayback "DreamReel/internal/interfaces/http/playback"
	interfaceshttprecommendation "DreamReel/internal/interfaces/http/recommendation"
	interfacehttprelation "DreamReel/internal/interfaces/http/relation"
	"DreamReel/internal/interfaces/http/router"
	interfacehttpvideo "DreamReel/internal/interfaces/http/video"

	"github.com/gin-gonic/gin"
)

const testInternalToken = "dreamreel-test-internal-token"

// testEnv 封装测试服务器及各模块内存仓储，便于注入测试数据与校验状态。
type testEnv struct {
	engine             *gin.Engine
	jwtManager         *jwt.Manager
	accountRepo        *memAccountRepo
	videoRepo          *memVideoRepo
	feedRepo           *memFeedRepo
	relationRepo       *memRelationRepo
	recommendationRepo *memRecommendationRepo
	messageRepo        *memMessageRepo
	playbackRepo       *memPlaybackRepo
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	gin.SetMode(gin.TestMode)

	jwtManager, err := jwt.NewManager("dreamreel-test-jwt-secret", "1h")
	if err != nil {
		t.Fatalf("初始化 JWT 管理器失败: %v", err)
	}

	accountRepo := newMemAccountRepo()
	videoRepo := newMemVideoRepo()
	feedRepo := newMemFeedRepo(videoRepo, accountRepo)
	relationRepo := newMemRelationRepo(accountRepo)
	recommendationRepo := newMemRecommendationRepo(videoRepo)
	messageRepo := newMemMessageRepo()
	playbackRepo := newMemPlaybackRepo(videoRepo)

	accountService := applicationaccount.New(accountRepo, jwtManager)
	accountHandler := interfaceshttpaccount.New(accountService)

	videoService := applicationvideo.New(videoRepo)
	videoHandler := interfacehttpvideo.New(videoService)

	messageService := applicationmessage.New(messageRepo)
	messageHandler := interfaceshttpmessage.New(messageService)
	messageWriter := router.NewMessageWriter(messageService)

	recommendationService := applicationrecommendation.New(recommendationRepo)
	recommendationHandler := interfaceshttprecommendation.New(recommendationService)

	feedService := applicationfeed.New(
		feedRepo,
		applicationfeed.WithRecommender(recommendationService),
	)
	feedHandler := interfaceshttpfeed.New(feedService)

	relationService := applicationrelation.New(
		relationRepo,
		applicationrelation.WithMessageWriter(messageWriter),
	)
	relationHandler := interfacehttprelation.New(relationService)

	playbackService := applicationplayback.New(playbackRepo)
	playbackHandler := interfacehttpplayback.New(playbackService)

	authMiddleware := middleware.NewJWTAuth(jwtManager)
	optionalAuthMiddleware := middleware.NewOptionalJWTAuth(jwtManager)
	internalAuthMiddleware := middleware.NewInternalTokenAuth(testInternalToken)

	engine := gin.New()
	api := engine.Group("/api")

	session := api.Group("/sessions")
	session.POST("", accountHandler.Login)
	session.DELETE("/current", authMiddleware, accountHandler.Logout)

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

	videos := api.Group("/videos")
	videos.POST("", authMiddleware, videoHandler.Create)
	videos.DELETE("/:videoId", authMiddleware, videoHandler.Delete)
	videos.GET("/:videoId", videoHandler.Get)

	api.GET("/feed-items", optionalAuthMiddleware, feedHandler.ListFeedItems)
	api.POST("/feed-queries", optionalAuthMiddleware, feedHandler.Query)

	api.GET("/messages", authMiddleware, messageHandler.List)
	api.PATCH("/messages", authMiddleware, messageHandler.MarkRead)
	api.GET("/message-stats/unread", authMiddleware, messageHandler.CountUnread)

	api.GET("/playback-config", authMiddleware, playbackHandler.GetConfig)
	api.GET("/preload-videos", authMiddleware, playbackHandler.ListPreloadVideos)
	api.POST("/playback-qos-reports", authMiddleware, playbackHandler.CreateQoSReport)

	internal := engine.Group("/internal")
	internal.POST("/recommendation-candidates", recommendationHandler.ListCandidates)
	internal.POST("/exposure-decisions", recommendationHandler.DecideExposures)
	internal.POST("/exposures", recommendationHandler.SaveExposures)
	internal.POST("/messages", internalAuthMiddleware, messageHandler.Create)
	internal.POST("/playback-qos-reports", internalAuthMiddleware, playbackHandler.CreateInternalQoSReport)

	return &testEnv{
		engine:             engine,
		jwtManager:         jwtManager,
		accountRepo:        accountRepo,
		videoRepo:          videoRepo,
		feedRepo:           feedRepo,
		relationRepo:       relationRepo,
		recommendationRepo: recommendationRepo,
		messageRepo:        messageRepo,
		playbackRepo:       playbackRepo,
	}
}

func doJSONRequest(t *testing.T, engine http.Handler, method, path string, body any, headers map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("序列化请求体失败: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	engine.ServeHTTP(rec, req)
	return rec
}

func (e *testEnv) registerAndLogin(t *testing.T, account, password, nickname string) (int64, string) {
	t.Helper()
	regRec := doJSONRequest(t, e.engine, http.MethodPost, "/api/users", map[string]any{
		"account":  account,
		"password": password,
		"nickname": nickname,
	}, nil)
	if regRec.Code != http.StatusCreated {
		t.Fatalf("注册失败: status=%d body=%s", regRec.Code, regRec.Body.String())
	}
	var regResp struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(regRec.Body.Bytes(), &regResp); err != nil {
		t.Fatalf("解析注册响应失败: %v", err)
	}

	loginRec := doJSONRequest(t, e.engine, http.MethodPost, "/api/sessions", map[string]any{
		"account":  account,
		"password": password,
	}, nil)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("登录失败: status=%d body=%s", loginRec.Code, loginRec.Body.String())
	}
	var loginResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(loginRec.Body.Bytes(), &loginResp); err != nil {
		t.Fatalf("解析登录响应失败: %v", err)
	}
	return regResp.ID, loginResp.AccessToken
}

// ============================================================================
// 1. Account 链路测试：GET /users/me、GET /users/:userId、PATCH /users/me
// ============================================================================

func TestAccountChains(t *testing.T) {
	env := newTestEnv(t)
	userID, token := env.registerAndLogin(t, "alice", "pass123456", "Alice")
	authHeader := map[string]string{"Authorization": "Bearer " + token}

	// 未登录访问 GET /api/users/me 返回 401
	unauthRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me", nil, nil)
	if unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("预期未登录返回 401，实际 %d", unauthRec.Code)
	}

	// 登录访问 GET /api/users/me 返回完整资料与计数
	env.accountRepo.setStats(userID, 3, 5, 2)
	meRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me", nil, authHeader)
	if meRec.Code != http.StatusOK {
		t.Fatalf("GET /api/users/me 失败: status=%d body=%s", meRec.Code, meRec.Body.String())
	}
	var meMap map[string]any
	_ = json.Unmarshal(meRec.Body.Bytes(), &meMap)
	if meMap["account"] != "alice" || meMap["nickname"] != "Alice" || int(meMap["following_count"].(float64)) != 3 || int(meMap["follower_count"].(float64)) != 5 || int(meMap["work_count"].(float64)) != 2 {
		t.Fatalf("GET /api/users/me 字段不符合预期: %+v", meMap)
	}

	// 公开资料 GET /api/users/:userId 隐藏敏感字段 account / status / role
	pubRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/1", nil, nil)
	if pubRec.Code != http.StatusOK {
		t.Fatalf("GET /api/users/1 失败: status=%d body=%s", pubRec.Code, pubRec.Body.String())
	}
	var pubMap map[string]any
	_ = json.Unmarshal(pubRec.Body.Bytes(), &pubMap)
	if _, hasAccount := pubMap["account"]; hasAccount {
		t.Fatalf("公开资料不应暴露 account 字段: %+v", pubMap)
	}
	if _, hasRole := pubMap["role"]; hasRole {
		t.Fatalf("公开资料不应暴露 role 字段: %+v", pubMap)
	}
	if _, hasStatus := pubMap["status"]; hasStatus {
		t.Fatalf("公开资料不应暴露 status 字段: %+v", pubMap)
	}

	// 非法路径参数与不存在用户
	if badRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/abc", nil, nil); badRec.Code != http.StatusBadRequest {
		t.Fatalf("预期非法 userId 返回 400，实际 %d", badRec.Code)
	}
	if notFoundRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/999", nil, nil); notFoundRec.Code != http.StatusNotFound {
		t.Fatalf("预期不存在用户返回 404，实际 %d", notFoundRec.Code)
	}

	// PATCH /api/users/me 空更新与空昵称拦截
	if emptyPatch := doJSONRequest(t, env.engine, http.MethodPatch, "/api/users/me", map[string]any{}, authHeader); emptyPatch.Code != http.StatusBadRequest {
		t.Fatalf("预期空更新返回 400，实际 %d", emptyPatch.Code)
	}
	if blankNick := doJSONRequest(t, env.engine, http.MethodPatch, "/api/users/me", map[string]any{"nickname": "   "}, authHeader); blankNick.Code != http.StatusBadRequest {
		t.Fatalf("预期空白昵称返回 400，实际 %d", blankNick.Code)
	}

	// PATCH /api/users/me 局部更新并支持置空简介
	patchRec := doJSONRequest(t, env.engine, http.MethodPatch, "/api/users/me", map[string]any{
		"nickname":   "AliceNew",
		"avatar_url": "https://cdn.example.com/a.png",
		"bio":        "AIGC 创作者",
	}, authHeader)
	if patchRec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/users/me 失败: status=%d body=%s", patchRec.Code, patchRec.Body.String())
	}

	clearBioRec := doJSONRequest(t, env.engine, http.MethodPatch, "/api/users/me", map[string]any{
		"bio": "",
	}, authHeader)
	if clearBioRec.Code != http.StatusOK {
		t.Fatalf("清空 bio 失败: status=%d body=%s", clearBioRec.Code, clearBioRec.Body.String())
	}
	var updatedProfile map[string]any
	_ = json.Unmarshal(clearBioRec.Body.Bytes(), &updatedProfile)
	if updatedProfile["nickname"] != "AliceNew" || updatedProfile["bio"] != "" {
		t.Fatalf("局部更新保留原昵称并清空 bio 不符合预期: %+v", updatedProfile)
	}

	// 被禁用账号登录返回 403 Forbidden
	env.accountRepo.setStatus(userID, 2)
	disabledLogin := doJSONRequest(t, env.engine, http.MethodPost, "/api/sessions", map[string]any{
		"account":  "alice",
		"password": "pass123456",
	}, nil)
	if disabledLogin.Code != http.StatusForbidden {
		t.Fatalf("预期禁用账号登录返回 403，实际 %d body=%s", disabledLogin.Code, disabledLogin.Body.String())
	}
}

// ============================================================================
// 2. Video 作品列表测试：GET /users/:userId/videos、GET /users/me/videos
// ============================================================================

func TestVideoListByAuthorAndMineChains(t *testing.T) {
	env := newTestEnv(t)
	authorID, token := env.registerAndLogin(t, "creator", "pass123456", "Creator")
	authHeader := map[string]string{"Authorization": "Bearer " + token}

	t1 := time.Date(2026, 5, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 5, 1, 11, 0, 0, 0, time.UTC)
	t3 := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	v1 := env.videoRepo.seedVideo(authorID, "作品1", t1, domainvideo.StatusPublished, 2, 1, 0)
	v2 := env.videoRepo.seedVideo(authorID, "作品2", t2, domainvideo.StatusPublished, 5, 2, 1)
	v3 := env.videoRepo.seedVideo(authorID, "作品3", t3, domainvideo.StatusPublished, 8, 3, 2)
	_ = env.videoRepo.seedVideo(authorID, "已删除作品", t3.Add(time.Hour), domainvideo.StatusDeleted, 0, 0, 0)

	// GET /api/users/:userId/videos 分页查询
	listRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/1/videos?limit=2&offset=0", nil, nil)
	if listRec.Code != http.StatusOK {
		t.Fatalf("ListByAuthor 失败: status=%d body=%s", listRec.Code, listRec.Body.String())
	}
	var page1 struct {
		Items  []struct{ ID int64 `json:"id"` } `json:"items"`
		Limit  int                              `json:"limit"`
		Offset int                              `json:"offset"`
	}
	_ = json.Unmarshal(listRec.Body.Bytes(), &page1)
	if len(page1.Items) != 2 || page1.Items[0].ID != v3.ID || page1.Items[1].ID != v2.ID {
		t.Fatalf("第一页排序不符合预期: %+v", page1)
	}

	// 第二页 offset=2
	listRec2 := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/1/videos?limit=2&offset=2", nil, nil)
	var page2 struct {
		Items []struct{ ID int64 `json:"id"` } `json:"items"`
	}
	_ = json.Unmarshal(listRec2.Body.Bytes(), &page2)
	if len(page2.Items) != 1 || page2.Items[0].ID != v1.ID {
		t.Fatalf("第二页结果不符合预期: %+v", page2)
	}

	// 非法分页参数拦截
	if badLimit := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/1/videos?limit=0", nil, nil); badLimit.Code != http.StatusBadRequest {
		t.Fatalf("预期 limit=0 返回 400，实际 %d", badLimit.Code)
	}
	if badOffset := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/1/videos?offset=-1", nil, nil); badOffset.Code != http.StatusBadRequest {
		t.Fatalf("预期 offset=-1 返回 400，实际 %d", badOffset.Code)
	}

	// GET /api/users/me/videos
	if unauthMine := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/videos", nil, nil); unauthMine.Code != http.StatusUnauthorized {
		t.Fatalf("预期未登录 ListMine 返回 401，实际 %d", unauthMine.Code)
	}
	mineRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/videos", nil, authHeader)
	if mineRec.Code != http.StatusOK {
		t.Fatalf("ListMine 失败: status=%d body=%s", mineRec.Code, mineRec.Body.String())
	}
	var mineResp struct {
		Items []struct{ ID int64 `json:"id"` } `json:"items"`
	}
	_ = json.Unmarshal(mineRec.Body.Bytes(), &mineResp)
	if len(mineResp.Items) != 3 {
		t.Fatalf("预期我的作品返回 3 条已发布视频，实际 %d", len(mineResp.Items))
	}
}

// ============================================================================
// 3. Feed 查询链路测试：POST /api/feed-queries
// ============================================================================

func TestFeedQueriesChains(t *testing.T) {
	env := newTestEnv(t)
	authorID, _ := env.registerAndLogin(t, "author1", "pass123456", "AuthorOne")
	viewerID, viewerToken := env.registerAndLogin(t, "viewer1", "pass123456", "ViewerOne")
	authHeader := map[string]string{"Authorization": "Bearer " + viewerToken}

	t1 := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	t2 := time.Date(2026, 6, 1, 11, 0, 0, 0, time.UTC)
	v1 := env.videoRepo.seedVideo(authorID, "AI 太空漫游", t1, domainvideo.StatusPublished, 10, 2, 1)
	v2 := env.videoRepo.seedVideo(authorID, "赛博城市霓虹", t2, domainvideo.StatusPublished, 1, 0, 0)
	env.feedRepo.setViewerAction(viewerID, v1.ID, true, false)
	env.feedRepo.addFollowing(viewerID, authorID)

	// 1. 默认空 scene 走 timeline，测试 limit=1 翻页
	q1Rec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{
		"limit": 1,
	}, nil)
	if q1Rec.Code != http.StatusOK {
		t.Fatalf("timeline 查询失败: status=%d body=%s", q1Rec.Code, q1Rec.Body.String())
	}
	var q1Resp struct {
		Scene      string `json:"scene"`
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
		Items      []struct {
			VideoID int64 `json:"video_id"`
		} `json:"items"`
	}
	_ = json.Unmarshal(q1Rec.Body.Bytes(), &q1Resp)
	if q1Resp.Scene != "timeline" || !q1Resp.HasMore || q1Resp.NextCursor == "" || len(q1Resp.Items) != 1 || q1Resp.Items[0].VideoID != v2.ID {
		t.Fatalf("timeline 第一页不符合预期: %+v", q1Resp)
	}

	// 使用游标拉取第二页
	q2Rec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{
		"scene":  "timeline",
		"cursor": q1Resp.NextCursor,
		"limit":  1,
	}, authHeader)
	var q2Resp struct {
		HasMore bool `json:"has_more"`
		Items   []struct {
			VideoID int64 `json:"video_id"`
			Liked   bool  `json:"liked"`
		} `json:"items"`
	}
	_ = json.Unmarshal(q2Rec.Body.Bytes(), &q2Resp)
	if q2Resp.HasMore || len(q2Resp.Items) != 1 || q2Resp.Items[0].VideoID != v1.ID || !q2Resp.Items[0].Liked {
		t.Fatalf("timeline 第二页及点赞水合状态不符合预期: %+v", q2Resp)
	}

	// 2. hot 场景按热度排序（v1 热度高于 v2）
	hotRec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{
		"scene": "hot",
		"limit": 2,
	}, nil)
	var hotResp struct {
		Scene string                          `json:"scene"`
		Items []struct{ VideoID int64 `json:"video_id"` } `json:"items"`
	}
	_ = json.Unmarshal(hotRec.Body.Bytes(), &hotResp)
	if hotResp.Scene != "hot" || len(hotResp.Items) != 2 || hotResp.Items[0].VideoID != v1.ID {
		t.Fatalf("hot 排序不符合预期: %+v", hotResp)
	}

	// 3. following 与 recommend 未登录返回 401，登录后返回 200
	if unauthFollow := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{"scene": "following"}, nil); unauthFollow.Code != http.StatusUnauthorized {
		t.Fatalf("未登录请求 following 预期返回 401，实际 %d", unauthFollow.Code)
	}
	if unauthRec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{"scene": "recommend"}, nil); unauthRec.Code != http.StatusUnauthorized {
		t.Fatalf("未登录请求 recommend 预期返回 401，实际 %d", unauthRec.Code)
	}

	followRec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{
		"scene": "following",
		"limit": 5,
	}, authHeader)
	var followResp struct {
		NextCursor string `json:"next_cursor"`
		HasMore    bool   `json:"has_more"`
		Items      []struct{ VideoID int64 `json:"video_id"` } `json:"items"`
	}
	_ = json.Unmarshal(followRec.Body.Bytes(), &followResp)
	if followRec.Code != http.StatusOK || len(followResp.Items) != 2 || followResp.HasMore || followResp.NextCursor != "" {
		t.Fatalf("following 最后一页结果不符合预期: code=%d resp=%+v", followRec.Code, followResp)
	}

	recSceneRec := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{
		"scene": "recommend",
		"limit": 5,
		"context": map[string]string{
			"request_id": "req-feed-rec-001",
		},
	}, authHeader)
	if recSceneRec.Code != http.StatusOK {
		t.Fatalf("recommend 查询失败: status=%d body=%s", recSceneRec.Code, recSceneRec.Body.String())
	}

	// 4. 非法 scene 返回 400
	if unsupported := doJSONRequest(t, env.engine, http.MethodPost, "/api/feed-queries", map[string]any{"scene": "invalid_scene"}, nil); unsupported.Code != http.StatusBadRequest {
		t.Fatalf("非法 scene 预期返回 400，实际 %d", unsupported.Code)
	}

	// 5. WithRecommender(nil) 安全返回 ErrUnsupportedScene 而不触发空指针 panic
	svcWithoutRec := applicationfeed.New(env.feedRepo, applicationfeed.WithRecommender(nil))
	if _, err := svcWithoutRec.GetFeed(context.Background(), applicationfeed.FeedRequest{
		Scene:    domainfeed.SceneRecommend,
		ViewerID: viewerID,
	}); err == nil {
		t.Fatalf("预期未注入推荐器时请求 recommend 返回错误")
	}
}

// ============================================================================
// 4. Relation 关注与粉丝列表测试：GET /users/me/following & followers
// ============================================================================

func TestRelationFollowingAndFollowersChains(t *testing.T) {
	env := newTestEnv(t)
	u1, token1 := env.registerAndLogin(t, "u1", "pass123456", "User1")
	u2, token2 := env.registerAndLogin(t, "u2", "pass123456", "User2")
	u3, token3 := env.registerAndLogin(t, "u3", "pass123456", "User3")

	h1 := map[string]string{"Authorization": "Bearer " + token1}
	h2 := map[string]string{"Authorization": "Bearer " + token2}
	h3 := map[string]string{"Authorization": "Bearer " + token3}

	// u1 依次关注 u2、u3；u2、u3 也关注 u1
	if r := doJSONRequest(t, env.engine, http.MethodPut, "/api/users/me/following/2", nil, h1); r.Code != http.StatusOK {
		t.Fatalf("u1 关注 u2 失败: %d %s", r.Code, r.Body.String())
	}
	time.Sleep(2 * time.Millisecond)
	if r := doJSONRequest(t, env.engine, http.MethodPut, "/api/users/me/following/3", nil, h1); r.Code != http.StatusOK {
		t.Fatalf("u1 关注 u3 失败: %d %s", r.Code, r.Body.String())
	}
	_ = u1
	if r := doJSONRequest(t, env.engine, http.MethodPut, "/api/users/me/following/1", nil, h2); r.Code != http.StatusOK {
		t.Fatalf("u2 关注 u1 失败: %d", r.Code)
	}
	time.Sleep(2 * time.Millisecond)
	if r := doJSONRequest(t, env.engine, http.MethodPut, "/api/users/me/following/1", nil, h3); r.Code != http.StatusOK {
		t.Fatalf("u3 关注 u1 失败: %d", r.Code)
	}

	// 分页查询关注列表：limit=1 第一页应为最新关注的 u3
	fg1Rec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/following?limit=1", nil, h1)
	if fg1Rec.Code != http.StatusOK {
		t.Fatalf("ListFollowing 第一页失败: %d %s", fg1Rec.Code, fg1Rec.Body.String())
	}
	var fg1 struct {
		Items      []struct{ UserID int64 `json:"user_id"` } `json:"items"`
		NextCursor string                                    `json:"next_cursor"`
		HasMore    bool                                      `json:"has_more"`
	}
	_ = json.Unmarshal(fg1Rec.Body.Bytes(), &fg1)
	if len(fg1.Items) != 1 || fg1.Items[0].UserID != u3 || !fg1.HasMore || fg1.NextCursor == "" {
		t.Fatalf("ListFollowing 第一页不符合预期: %+v", fg1)
	}

	// 第二页应为 u2，且 has_more=false，next_cursor 指向当页末尾记录
	fg2Rec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/following?limit=1&cursor="+fg1.NextCursor, nil, h1)
	var fg2 struct {
		Items      []struct{ UserID int64 `json:"user_id"` } `json:"items"`
		NextCursor string                                    `json:"next_cursor"`
		HasMore    bool                                      `json:"has_more"`
	}
	_ = json.Unmarshal(fg2Rec.Body.Bytes(), &fg2)
	if len(fg2.Items) != 1 || fg2.Items[0].UserID != u2 || fg2.HasMore || fg2.NextCursor == "" {
		t.Fatalf("ListFollowing 第二页不符合预期: %+v", fg2)
	}

	// 查询粉丝列表：u1 的粉丝第一页应为最新关注的 u3，第二页为 u2
	fl1Rec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/followers?limit=1", nil, h1)
	var fl1 struct {
		Items      []struct{ UserID int64 `json:"user_id"` } `json:"items"`
		NextCursor string                                    `json:"next_cursor"`
		HasMore    bool                                      `json:"has_more"`
	}
	_ = json.Unmarshal(fl1Rec.Body.Bytes(), &fl1)
	if fl1Rec.Code != http.StatusOK || len(fl1.Items) != 1 || fl1.Items[0].UserID != u3 || !fl1.HasMore {
		t.Fatalf("ListFollowers 第一页不符合预期: %+v", fl1)
	}

	fl2Rec := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/followers?limit=1&cursor="+fl1.NextCursor, nil, h1)
	var fl2 struct {
		Items      []struct{ UserID int64 `json:"user_id"` } `json:"items"`
		NextCursor string                                    `json:"next_cursor"`
		HasMore    bool                                      `json:"has_more"`
	}
	_ = json.Unmarshal(fl2Rec.Body.Bytes(), &fl2)
	if len(fl2.Items) != 1 || fl2.Items[0].UserID != u2 || fl2.HasMore || fl2.NextCursor == "" {
		t.Fatalf("ListFollowers 第二页不符合预期: %+v", fl2)
	}

	// 非法参数与未登录校验
	if unauth := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/following", nil, nil); unauth.Code != http.StatusUnauthorized {
		t.Fatalf("预期未登录返回 401，实际 %d", unauth.Code)
	}
	if badLim := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/following?limit=0", nil, h1); badLim.Code != http.StatusBadRequest {
		t.Fatalf("预期 limit=0 返回 400，实际 %d", badLim.Code)
	}
	if badCur := doJSONRequest(t, env.engine, http.MethodGet, "/api/users/me/followers?cursor=invalid", nil, h1); badCur.Code != http.StatusBadRequest {
		t.Fatalf("预期非法 cursor 返回 400，实际 %d", badCur.Code)
	}
}

// ============================================================================
// 5. /internal 推荐候选与曝光链路测试
// ============================================================================

func TestInternalRecommendationAndExposureChains(t *testing.T) {
	env := newTestEnv(t)
	a1, _ := env.registerAndLogin(t, "rec_a1", "pass123456", "RecAuthor1")
	a2, _ := env.registerAndLogin(t, "rec_a2", "pass123456", "RecAuthor2")

	now := time.Now().UTC()
	v1 := env.videoRepo.seedVideo(a1, "星际飞船建模教程", now.Add(-1*time.Hour), domainvideo.StatusPublished, 20, 5, 3)
	v2 := env.videoRepo.seedVideo(a1, "星际行星渲染技巧", now.Add(-2*time.Hour), domainvideo.StatusPublished, 18, 4, 2)
	v3 := env.videoRepo.seedVideo(a2, "水墨国风动画短片", now.Add(-3*time.Hour), domainvideo.StatusPublished, 5, 1, 0)

	// 1. POST /internal/recommendation-candidates（验证召回排序与同作者打散）
	candRec := doJSONRequest(t, env.engine, http.MethodPost, "/internal/recommendation-candidates", map[string]any{
		"user_id":    100,
		"scene":      "recommend",
		"request_id": "req-cand-1",
		"limit":      3,
	}, nil)
	if candRec.Code != http.StatusOK {
		t.Fatalf("候选召回失败: status=%d body=%s", candRec.Code, candRec.Body.String())
	}
	var candResp struct {
		Candidates []struct {
			VideoID  int64 `json:"video_id"`
			AuthorID int64 `json:"author_id"`
		} `json:"candidates"`
	}
	_ = json.Unmarshal(candRec.Body.Bytes(), &candResp)
	if len(candResp.Candidates) != 3 {
		t.Fatalf("预期召回 3 条候选视频，实际 %d", len(candResp.Candidates))
	}
	// a1 的两条高分视频中间应穿插 a2 的视频，避免连续同作者刷屏
	if candResp.Candidates[0].AuthorID == candResp.Candidates[1].AuthorID {
		t.Fatalf("预期相邻同作者视频被打散，实际顺序: %+v", candResp.Candidates)
	}

	// 2. POST /internal/exposures 写入曝光与视频不存在回滚校验
	notFoundExp := doJSONRequest(t, env.engine, http.MethodPost, "/internal/exposures", map[string]any{
		"user_id":   100,
		"scene":     "recommend",
		"video_ids": []int64{v1.ID, 99999},
	}, nil)
	if notFoundExp.Code != http.StatusNotFound {
		t.Fatalf("包含不存在视频预期返回 404，实际 %d", notFoundExp.Code)
	}

	saveExp1 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/exposures", map[string]any{
		"user_id":    100,
		"scene":      "recommend",
		"request_id": "req-exp-1",
		"video_ids":  []int64{v1.ID, v1.ID},
	}, nil)
	if saveExp1.Code != http.StatusCreated {
		t.Fatalf("写入曝光失败: status=%d body=%s", saveExp1.Code, saveExp1.Body.String())
	}
	// 再次曝光累加 exposure_count
	saveExp2 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/exposures", map[string]any{
		"user_id":    100,
		"scene":      "recommend",
		"request_id": "req-exp-2",
		"video_ids":  []int64{v1.ID},
	}, nil)
	var expResp struct {
		Exposures []struct {
			VideoID       int64 `json:"video_id"`
			ExposureCount int   `json:"exposure_count"`
		} `json:"exposures"`
	}
	_ = json.Unmarshal(saveExp2.Body.Bytes(), &expResp)
	if len(expResp.Exposures) != 1 || expResp.Exposures[0].ExposureCount != 2 {
		t.Fatalf("预期重复曝光后 exposure_count=2，实际 %+v", expResp)
	}

	// 3. POST /internal/exposure-decisions 判定近期已曝光与未曝光视频
	decRec := doJSONRequest(t, env.engine, http.MethodPost, "/internal/exposure-decisions", map[string]any{
		"user_id":   100,
		"scene":     "recommend",
		"video_ids": []int64{v1.ID, v2.ID, v3.ID},
	}, nil)
	if decRec.Code != http.StatusOK {
		t.Fatalf("曝光决策失败: status=%d body=%s", decRec.Code, decRec.Body.String())
	}
	var decResp struct {
		Decisions []struct {
			VideoID int64  `json:"video_id"`
			Allowed bool   `json:"allowed"`
			Reason  string `json:"reason"`
		} `json:"decisions"`
	}
	_ = json.Unmarshal(decRec.Body.Bytes(), &decResp)
	if len(decResp.Decisions) != 3 || decResp.Decisions[0].Allowed || decResp.Decisions[0].Reason != "recently_exposed" || !decResp.Decisions[1].Allowed || decResp.Decisions[1].Reason != "fresh" {
		t.Fatalf("曝光决策结果不符合预期: %+v", decResp.Decisions)
	}
}

// ============================================================================
// 6. /internal 消息写入与播放 QoS 上报及默认配置回退测试
// ============================================================================

func TestInternalMessagesAndPlaybackChains(t *testing.T) {
	env := newTestEnv(t)
	userID, userToken := env.registerAndLogin(t, "msg_user", "pass123456", "MsgUser")
	video := env.videoRepo.seedVideo(userID, "测试视频", time.Now().UTC(), domainvideo.StatusPublished, 0, 0, 0)

	userAuth := map[string]string{"Authorization": "Bearer " + userToken}
	internalHeaders := map[string]string{"X-Internal-Token": testInternalToken}

	// 1. 内部鉴权拦截校验
	if noTok := doJSONRequest(t, env.engine, http.MethodPost, "/internal/messages", map[string]any{}, nil); noTok.Code != http.StatusUnauthorized {
		t.Fatalf("缺少 X-Internal-Token 预期返回 401，实际 %d", noTok.Code)
	}
	if badTok := doJSONRequest(t, env.engine, http.MethodPost, "/internal/playback-qos-reports", map[string]any{}, map[string]string{"X-Internal-Token": "wrong-token"}); badTok.Code != http.StatusUnauthorized {
		t.Fatalf("错误 X-Internal-Token 预期返回 401，实际 %d", badTok.Code)
	}

	// 2. POST /internal/messages 首次创建返回 201，幂等重试返回 200
	msgPayload := map[string]any{
		"user_id":          userID,
		"type":             "LIKE",
		"title":            "收到新赞",
		"content":          "Bob 赞了你的作品",
		"event_id":         "evt-like-1001",
		"actor_id":         99,
		"actor_nickname":   "Bob",
		"actor_avatar_url": "https://cdn.example.com/bob.png",
	}
	createMsg1 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/messages", msgPayload, internalHeaders)
	if createMsg1.Code != http.StatusCreated {
		t.Fatalf("首次创建内部消息预期 201，实际 %d body=%s", createMsg1.Code, createMsg1.Body.String())
	}
	var msg1 struct {
		ID            int64  `json:"id"`
		ActorNickname string `json:"actor_nickname"`
	}
	_ = json.Unmarshal(createMsg1.Body.Bytes(), &msg1)

	createMsg2 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/messages", msgPayload, internalHeaders)
	if createMsg2.Code != http.StatusOK {
		t.Fatalf("重复 event_id 创建内部消息预期幂等返回 200，实际 %d", createMsg2.Code)
	}
	var msg2 struct {
		ID int64 `json:"id"`
	}
	_ = json.Unmarshal(createMsg2.Body.Bytes(), &msg2)
	if msg1.ID != msg2.ID || msg1.ActorNickname != "Bob" {
		t.Fatalf("幂等消息 ID 或触发人昵称不匹配: msg1=%+v msg2=%+v", msg1, msg2)
	}

	// 验证 PATCH /api/messages 传空对象 {} 标记全部未读消息为已读
	markAllRec := doJSONRequest(t, env.engine, http.MethodPatch, "/api/messages", map[string]any{}, userAuth)
	if markAllRec.Code != http.StatusOK {
		t.Fatalf("PATCH /api/messages 全部标为已读失败: %d %s", markAllRec.Code, markAllRec.Body.String())
	}
	var markResp struct {
		UpdatedCount int `json:"updated_count"`
		UnreadCount  int `json:"unread_count"`
	}
	_ = json.Unmarshal(markAllRec.Body.Bytes(), &markResp)
	if markResp.UpdatedCount != 1 || markResp.UnreadCount != 0 {
		t.Fatalf("全部已读更新计数不符合预期: %+v", markResp)
	}

	// 3. POST /internal/playback-qos-reports 首次 201、幂等 200
	qosHeaders := map[string]string{
		"X-Internal-Token": testInternalToken,
		"Idempotency-Key":  "qos-idem-001",
	}
	qosPayload := map[string]any{
		"user_id":        userID,
		"video_id":       video.ID,
		"first_frame_ms": 180,
		"stutter_count":  0,
		"watch_ms":       12000,
	}
	qosRec1 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/playback-qos-reports", qosPayload, qosHeaders)
	if qosRec1.Code != http.StatusCreated {
		t.Fatalf("首次内部上报 QoS 预期 201，实际 %d body=%s", qosRec1.Code, qosRec1.Body.String())
	}
	qosRec2 := doJSONRequest(t, env.engine, http.MethodPost, "/internal/playback-qos-reports", qosPayload, qosHeaders)
	if qosRec2.Code != http.StatusOK {
		t.Fatalf("幂等重放内部 QoS 预期 200，实际 %d body=%s", qosRec2.Code, qosRec2.Body.String())
	}

	// 4. 验证播放配置按平台 DEFAULT 网络回退逻辑
	env.playbackRepo.seedConfig("iOS", domainplayback.NetworkDefault, 6, 2400)
	cfgRec := doJSONRequest(t, env.engine, http.MethodGet, "/api/playback-config?platform=iOS&network_type=5G", nil, userAuth)
	if cfgRec.Code != http.StatusOK {
		t.Fatalf("获取播放配置失败: %d %s", cfgRec.Code, cfgRec.Body.String())
	}
	var cfgResp struct {
		Platform     string `json:"platform"`
		NetworkType  string `json:"network_type"`
		PreloadCount int    `json:"preload_count"`
		BufferMs     int    `json:"buffer_ms"`
	}
	_ = json.Unmarshal(cfgRec.Body.Bytes(), &cfgResp)
	if cfgResp.Platform != "iOS" || cfgResp.PreloadCount != 6 || cfgResp.BufferMs != 2400 {
		t.Fatalf("预期 iOS/5G 未单独配置时回退命中 iOS/DEFAULT(preload=6, buffer=2400)，实际 %+v", cfgResp)
	}
}

// ============================================================================
// 内存仓储实现（仅用于测试隔离，模拟真实持久化契约）
// ============================================================================

type memAccountRepo struct {
	mu        sync.Mutex
	nextID    int64
	byID      map[int64]*domainaccount.User
	byAccount map[string]int64
}

func newMemAccountRepo() *memAccountRepo {
	return &memAccountRepo{
		nextID:    1,
		byID:      map[int64]*domainaccount.User{},
		byAccount: map[string]int64{},
	}
}

func cloneUser(u *domainaccount.User) *domainaccount.User {
	if u == nil {
		return nil
	}
	copied := *u
	return &copied
}

func (r *memAccountRepo) Save(_ context.Context, user *domainaccount.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.byAccount[user.Account]; ok {
		return domainaccount.ErrAccountAlreadyExists
	}
	user.ID = r.nextID
	r.nextID++
	r.byID[user.ID] = cloneUser(user)
	r.byAccount[user.Account] = user.ID
	return nil
}

func (r *memAccountRepo) FindByAccount(_ context.Context, account string) (*domainaccount.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	id, ok := r.byAccount[account]
	if !ok {
		return nil, domainaccount.ErrUserNotFound
	}
	return cloneUser(r.byID[id]), nil
}

func (r *memAccountRepo) FindByID(_ context.Context, id int64) (*domainaccount.User, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	u, ok := r.byID[id]
	if !ok {
		return nil, domainaccount.ErrUserNotFound
	}
	return cloneUser(u), nil
}

func (r *memAccountRepo) UpdateProfile(_ context.Context, user *domainaccount.User) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.byID[user.ID]
	if !ok {
		return domainaccount.ErrUserNotFound
	}
	existing.Nickname = user.Nickname
	existing.AvatarURL = user.AvatarURL
	existing.Bio = user.Bio
	return nil
}

func (r *memAccountRepo) setStats(id int64, following, follower, works int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.FollowingCount = following
		u.FollowerCount = follower
		u.WorkCount = works
	}
}

func (r *memAccountRepo) setStatus(id int64, status int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if u, ok := r.byID[id]; ok {
		u.Status = status
	}
}

type memVideoRepo struct {
	mu     sync.Mutex
	nextID int64
	videos map[int64]*domainvideo.Video
}

func newMemVideoRepo() *memVideoRepo {
	return &memVideoRepo{
		nextID: 1,
		videos: map[int64]*domainvideo.Video{},
	}
}

func cloneVideo(v *domainvideo.Video) *domainvideo.Video {
	if v == nil {
		return nil
	}
	copied := *v
	return &copied
}

func (r *memVideoRepo) seedVideo(authorID int64, title string, publishedAt time.Time, status, likes, comments, favs int) *domainvideo.Video {
	r.mu.Lock()
	defer r.mu.Unlock()
	t := publishedAt.UTC()
	v := &domainvideo.Video{
		ID:            r.nextID,
		AuthorID:      authorID,
		Title:         title,
		Description:   title + " 描述",
		MediaURL:      "https://cdn.example.com/v.mp4",
		CoverURL:      "https://cdn.example.com/c.jpg",
		Status:        status,
		LikeCount:     likes,
		CommentCount:  comments,
		FavoriteCount: favs,
		PublishedAt:   &t,
		CreatedAt:     t,
		UpdatedAt:     t,
	}
	r.nextID++
	r.videos[v.ID] = v
	return cloneVideo(v)
}

func (r *memVideoRepo) Save(_ context.Context, video *domainvideo.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	video.ID = r.nextID
	r.nextID++
	r.videos[video.ID] = cloneVideo(video)
	return nil
}

func (r *memVideoRepo) FindByAuthorAndIdempotencyKey(_ context.Context, authorID int64, key string) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, v := range r.videos {
		if v.AuthorID == authorID && v.IdempotencyKey != nil && *v.IdempotencyKey == key {
			return cloneVideo(v), nil
		}
	}
	return nil, domainvideo.ErrVideoNotFound
}

func (r *memVideoRepo) FindByIDAnyStatus(_ context.Context, id int64) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.videos[id]
	if !ok {
		return nil, domainvideo.ErrVideoNotFound
	}
	return cloneVideo(v), nil
}

func (r *memVideoRepo) UpdateStatus(_ context.Context, video *domainvideo.Video) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.videos[video.ID]
	if !ok {
		return domainvideo.ErrVideoNotFound
	}
	v.Status = video.Status
	return nil
}

func (r *memVideoRepo) FindByID(_ context.Context, id int64) (*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	v, ok := r.videos[id]
	if !ok || v.Status != domainvideo.StatusPublished {
		return nil, domainvideo.ErrVideoNotFound
	}
	return cloneVideo(v), nil
}

func (r *memVideoRepo) ListByAuthor(_ context.Context, authorID int64, limit, offset int) ([]*domainvideo.Video, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*domainvideo.Video
	for _, v := range r.videos {
		if v.AuthorID == authorID && v.Status == domainvideo.StatusPublished && v.PublishedAt != nil {
			list = append(list, cloneVideo(v))
		}
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].PublishedAt.Equal(*list[j].PublishedAt) {
			return list[i].ID > list[j].ID
		}
		return list[i].PublishedAt.After(*list[j].PublishedAt)
	})
	if offset >= len(list) {
		return []*domainvideo.Video{}, nil
	}
	end := offset + limit
	if end > len(list) {
		end = len(list)
	}
	return list[offset:end], nil
}

type memFeedRepo struct {
	mu            sync.Mutex
	videoRepo     *memVideoRepo
	accountRepo   *memAccountRepo
	viewerActions map[int64]map[int64]*domainfeed.ViewerActionState
	following     map[int64]map[int64]bool
}

func newMemFeedRepo(videoRepo *memVideoRepo, accountRepo *memAccountRepo) *memFeedRepo {
	return &memFeedRepo{
		videoRepo:     videoRepo,
		accountRepo:   accountRepo,
		viewerActions: map[int64]map[int64]*domainfeed.ViewerActionState{},
		following:     map[int64]map[int64]bool{},
	}
}

func (r *memFeedRepo) setViewerAction(viewerID, videoID int64, liked, favorited bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.viewerActions[viewerID] == nil {
		r.viewerActions[viewerID] = map[int64]*domainfeed.ViewerActionState{}
	}
	r.viewerActions[viewerID][videoID] = &domainfeed.ViewerActionState{
		VideoID:   videoID,
		Liked:     liked,
		Favorited: favorited,
	}
}

func (r *memFeedRepo) addFollowing(viewerID, authorID int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.following[viewerID] == nil {
		r.following[viewerID] = map[int64]bool{}
	}
	r.following[viewerID][authorID] = true
}

func (r *memFeedRepo) publishedVideos() []*domainvideo.Video {
	r.videoRepo.mu.Lock()
	defer r.videoRepo.mu.Unlock()
	var list []*domainvideo.Video
	for _, v := range r.videoRepo.videos {
		if v.Status == domainvideo.StatusPublished && v.PublishedAt != nil {
			list = append(list, cloneVideo(v))
		}
	}
	return list
}

func (r *memFeedRepo) ListTimelinePage(_ context.Context, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	videos := r.publishedVideos()
	sort.Slice(videos, func(i, j int) bool {
		if videos[i].PublishedAt.Equal(*videos[j].PublishedAt) {
			return videos[i].ID > videos[j].ID
		}
		return videos[i].PublishedAt.After(*videos[j].PublishedAt)
	})
	var items []*domainfeed.FeedPageItem
	for _, v := range videos {
		if cursor != nil {
			if v.PublishedAt.After(cursor.PublishedAt) || (v.PublishedAt.Equal(cursor.PublishedAt) && v.ID >= cursor.VideoID) {
				continue
			}
		}
		items = append(items, &domainfeed.FeedPageItem{
			VideoID:     v.ID,
			AuthorID:    v.AuthorID,
			PublishedAt: *v.PublishedAt,
		})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (r *memFeedRepo) BatchGetFeedCards(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedCard, error) {
	cards := map[int64]*domainfeed.FeedCard{}
	for _, id := range videoIDs {
		v, err := r.videoRepo.FindByID(ctx, id)
		if err != nil {
			continue
		}
		nick := ""
		avatar := ""
		if u, err := r.accountRepo.FindByID(ctx, v.AuthorID); err == nil {
			nick = u.Nickname
			avatar = u.AvatarURL
		}
		cards[id] = &domainfeed.FeedCard{
			VideoID:         v.ID,
			AuthorID:        v.AuthorID,
			AuthorNickname:  nick,
			AuthorAvatarURL: avatar,
			Title:           v.Title,
			Description:     v.Description,
			MediaURL:        v.MediaURL,
			CoverURL:        v.CoverURL,
			PublishedAt:     *v.PublishedAt,
			ModelName:       v.ModelName,
		}
	}
	return cards, nil
}

func (r *memFeedRepo) BatchGetFeedStats(ctx context.Context, videoIDs []int64) (map[int64]*domainfeed.FeedStat, error) {
	stats := map[int64]*domainfeed.FeedStat{}
	for _, id := range videoIDs {
		stats[id] = &domainfeed.FeedStat{VideoID: id}
		if v, err := r.videoRepo.FindByID(ctx, id); err == nil {
			stats[id] = &domainfeed.FeedStat{
				VideoID:       id,
				LikeCount:     v.LikeCount,
				CommentCount:  v.CommentCount,
				FavoriteCount: v.FavoriteCount,
			}
		}
	}
	return stats, nil
}

func (r *memFeedRepo) BatchGetViewerActions(_ context.Context, viewerID int64, videoIDs []int64) (map[int64]*domainfeed.ViewerActionState, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	res := map[int64]*domainfeed.ViewerActionState{}
	for _, id := range videoIDs {
		res[id] = &domainfeed.ViewerActionState{VideoID: id}
		if m := r.viewerActions[viewerID]; m != nil && m[id] != nil {
			copied := *m[id]
			res[id] = &copied
		}
	}
	return res, nil
}

func (r *memFeedRepo) ListHotPage(_ context.Context, cursor *domainfeed.HotCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	videos := r.publishedVideos()
	type scored struct {
		v     *domainvideo.Video
		score int
	}
	list := make([]scored, 0, len(videos))
	for _, v := range videos {
		list = append(list, scored{
			v:     v,
			score: domainfeed.ScoreHotFeedItem(v.LikeCount, v.CommentCount, v.FavoriteCount),
		})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].score != list[j].score {
			return list[i].score > list[j].score
		}
		if !list[i].v.PublishedAt.Equal(*list[j].v.PublishedAt) {
			return list[i].v.PublishedAt.After(*list[j].v.PublishedAt)
		}
		return list[i].v.ID > list[j].v.ID
	})
	var items []*domainfeed.FeedPageItem
	for _, s := range list {
		if cursor != nil {
			if s.score > cursor.HotScore ||
				(s.score == cursor.HotScore && s.v.PublishedAt.After(cursor.PublishedAt)) ||
				(s.score == cursor.HotScore && s.v.PublishedAt.Equal(cursor.PublishedAt) && s.v.ID >= cursor.VideoID) {
				continue
			}
		}
		items = append(items, &domainfeed.FeedPageItem{
			VideoID:     s.v.ID,
			AuthorID:    s.v.AuthorID,
			PublishedAt: *s.v.PublishedAt,
			HotScore:    s.score,
		})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

func (r *memFeedRepo) ListFollowingPullAuthorIDs(_ context.Context, _ int64) ([]int64, error) {
	return nil, nil
}

func (r *memFeedRepo) ListFollowingPage(_ context.Context, viewerID int64, cursor *domainfeed.TimelineCursor, limit int) ([]*domainfeed.FeedPageItem, error) {
	r.mu.Lock()
	followed := map[int64]bool{}
	for k, v := range r.following[viewerID] {
		followed[k] = v
	}
	r.mu.Unlock()

	videos := r.publishedVideos()
	sort.Slice(videos, func(i, j int) bool {
		if videos[i].PublishedAt.Equal(*videos[j].PublishedAt) {
			return videos[i].ID > videos[j].ID
		}
		return videos[i].PublishedAt.After(*videos[j].PublishedAt)
	})
	var items []*domainfeed.FeedPageItem
	for _, v := range videos {
		if !followed[v.AuthorID] {
			continue
		}
		if cursor != nil {
			if v.PublishedAt.After(cursor.PublishedAt) || (v.PublishedAt.Equal(cursor.PublishedAt) && v.ID >= cursor.VideoID) {
				continue
			}
		}
		items = append(items, &domainfeed.FeedPageItem{
			VideoID:     v.ID,
			AuthorID:    v.AuthorID,
			PublishedAt: *v.PublishedAt,
		})
		if len(items) >= limit {
			break
		}
	}
	return items, nil
}

type memRelationRepo struct {
	mu          sync.Mutex
	accountRepo *memAccountRepo
	follows     map[[2]int64]*domainrelation.Follow
}

func newMemRelationRepo(accountRepo *memAccountRepo) *memRelationRepo {
	return &memRelationRepo{
		accountRepo: accountRepo,
		follows:     map[[2]int64]*domainrelation.Follow{},
	}
}

func (r *memRelationRepo) SetFollow(ctx context.Context, userID int64, targetUserID int64, active bool, idempotencyKey string) (*domainrelation.Follow, *domainrelation.RelationStat, *domainrelation.RelationStat, int, error) {
	if _, err := r.accountRepo.FindByID(ctx, targetUserID); err != nil {
		return nil, nil, nil, 0, domainrelation.ErrTargetUserNotFound
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	key := [2]int64{userID, targetUserID}
	now := time.Now().UTC()
	existing := r.follows[key]
	delta := 0
	status := domainrelation.FollowStatusCanceled
	if active {
		status = domainrelation.FollowStatusActive
	}
	if existing == nil {
		if active {
			delta = 1
		}
		existing = &domainrelation.Follow{
			ID:             int64(len(r.follows) + 1),
			UserID:         userID,
			TargetUserID:   targetUserID,
			Status:         status,
			IdempotencyKey: idempotencyKey,
			CreatedAt:      now,
			UpdatedAt:      now,
		}
		r.follows[key] = existing
	} else {
		if existing.Active() && !active {
			delta = -1
		} else if !existing.Active() && active {
			delta = 1
		}
		existing.Status = status
		existing.UpdatedAt = now
	}
	followingCount := 0
	followerCount := 0
	for _, f := range r.follows {
		if f.Status == domainrelation.FollowStatusActive {
			if f.UserID == userID {
				followingCount++
			}
			if f.TargetUserID == targetUserID {
				followerCount++
			}
		}
	}
	userStat := &domainrelation.RelationStat{UserID: userID, FollowingCount: followingCount, UpdatedAt: now}
	targetStat := &domainrelation.RelationStat{UserID: targetUserID, FollowerCount: followerCount, UpdatedAt: now}
	copied := *existing
	return &copied, userStat, targetStat, delta, nil
}

func (r *memRelationRepo) GetUserProfile(ctx context.Context, userID int64) (*domainrelation.UserProfile, error) {
	u, err := r.accountRepo.FindByID(ctx, userID)
	if err != nil {
		return nil, domainrelation.ErrTargetUserNotFound
	}
	return &domainrelation.UserProfile{
		UserID:    u.ID,
		Nickname:  u.Nickname,
		AvatarURL: u.AvatarURL,
		Bio:       u.Bio,
	}, nil
}

func (r *memRelationRepo) ListFollowing(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	r.mu.Lock()
	var rows []*domainrelation.Follow
	for _, f := range r.follows {
		if f.UserID == userID && f.Status == domainrelation.FollowStatusActive {
			copied := *f
			rows = append(rows, &copied)
		}
	}
	r.mu.Unlock()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].TargetUserID > rows[j].TargetUserID
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})

	var res []*domainrelation.UserItem
	for _, f := range rows {
		if cursor != nil {
			if f.UpdatedAt.After(cursor.FollowedAt) || (f.UpdatedAt.Equal(cursor.FollowedAt) && f.TargetUserID >= cursor.UserID) {
				continue
			}
		}
		u, err := r.accountRepo.FindByID(ctx, f.TargetUserID)
		if err != nil || u.Status != domainaccount.StatusNormal {
			continue
		}
		res = append(res, &domainrelation.UserItem{
			UserID:     u.ID,
			Nickname:   u.Nickname,
			AvatarURL:  u.AvatarURL,
			Bio:        u.Bio,
			FollowedAt: f.UpdatedAt,
		})
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}

func (r *memRelationRepo) ListFollowers(ctx context.Context, userID int64, cursor *domainrelation.ListCursor, limit int) ([]*domainrelation.UserItem, error) {
	r.mu.Lock()
	var rows []*domainrelation.Follow
	for _, f := range r.follows {
		if f.TargetUserID == userID && f.Status == domainrelation.FollowStatusActive {
			copied := *f
			rows = append(rows, &copied)
		}
	}
	r.mu.Unlock()

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].UpdatedAt.Equal(rows[j].UpdatedAt) {
			return rows[i].UserID > rows[j].UserID
		}
		return rows[i].UpdatedAt.After(rows[j].UpdatedAt)
	})

	var res []*domainrelation.UserItem
	for _, f := range rows {
		if cursor != nil {
			if f.UpdatedAt.After(cursor.FollowedAt) || (f.UpdatedAt.Equal(cursor.FollowedAt) && f.UserID >= cursor.UserID) {
				continue
			}
		}
		u, err := r.accountRepo.FindByID(ctx, f.UserID)
		if err != nil || u.Status != domainaccount.StatusNormal {
			continue
		}
		res = append(res, &domainrelation.UserItem{
			UserID:     u.ID,
			Nickname:   u.Nickname,
			AvatarURL:  u.AvatarURL,
			Bio:        u.Bio,
			FollowedAt: f.UpdatedAt,
		})
		if len(res) >= limit {
			break
		}
	}
	return res, nil
}

type memRecommendationRepo struct {
	mu        sync.Mutex
	videoRepo *memVideoRepo
	exposures map[[2]int64]*domainrecommendation.Exposure
}

func newMemRecommendationRepo(videoRepo *memVideoRepo) *memRecommendationRepo {
	return &memRecommendationRepo{
		videoRepo: videoRepo,
		exposures: map[[2]int64]*domainrecommendation.Exposure{},
	}
}

func (r *memRecommendationRepo) ListCandidatePool(_ context.Context, userID int64, limit int) ([]*domainrecommendation.Candidate, error) {
	r.videoRepo.mu.Lock()
	var videos []*domainvideo.Video
	for _, v := range r.videoRepo.videos {
		if v.Status == domainvideo.StatusPublished && v.PublishedAt != nil {
			videos = append(videos, cloneVideo(v))
		}
	}
	r.videoRepo.mu.Unlock()

	r.mu.Lock()
	defer r.mu.Unlock()
	cutoff := time.Now().UTC().Add(-domainrecommendation.RecentExposureWindow)
	var out []*domainrecommendation.Candidate
	for _, v := range videos {
		if exp := r.exposures[[2]int64{userID, v.ID}]; exp != nil && !exp.LastExposedAt.Before(cutoff) {
			continue
		}
		hot := domainfeed.ScoreHotFeedItem(v.LikeCount, v.CommentCount, v.FavoriteCount)
		out = append(out, &domainrecommendation.Candidate{
			VideoID:     v.ID,
			AuthorID:    v.AuthorID,
			HotScore:    hot,
			PublishedAt: *v.PublishedAt,
		})
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

func (r *memRecommendationRepo) LoadVideoVectors(_ context.Context, videoIDs []int64) (map[int64][]float64, error) {
	res := map[int64][]float64{}
	for _, id := range videoIDs {
		res[id] = []float64{1, 0, 0, 0}
	}
	return res, nil
}

func (r *memRecommendationRepo) LoadUserInterestVector(_ context.Context, _ int64) ([]float64, bool, error) {
	return nil, false, nil
}

func (r *memRecommendationRepo) ListRecentBehaviorSamples(_ context.Context, _ int64, _ string, _ time.Time, _ int) ([]*domainrecommendation.BehaviorSample, error) {
	return nil, nil
}

func (r *memRecommendationRepo) SaveUserInterestProfile(_ context.Context, _ *domainrecommendation.UserInterestProfile) error {
	return nil
}

func (r *memRecommendationRepo) ListRecentExposures(_ context.Context, userID int64, videoIDs []int64, since time.Time) ([]*domainrecommendation.Exposure, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var res []*domainrecommendation.Exposure
	for _, vid := range videoIDs {
		if exp := r.exposures[[2]int64{userID, vid}]; exp != nil && !exp.LastExposedAt.Before(since) {
			copied := *exp
			res = append(res, &copied)
		}
	}
	return res, nil
}

func (r *memRecommendationRepo) SaveExposures(ctx context.Context, writes []*domainrecommendation.ExposureWrite) ([]*domainrecommendation.Exposure, error) {
	for _, w := range writes {
		if _, err := r.videoRepo.FindByID(ctx, w.VideoID); err != nil {
			return nil, domainrecommendation.ErrVideoNotFound
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now().UTC()
	var res []*domainrecommendation.Exposure
	for _, w := range writes {
		key := [2]int64{w.UserID, w.VideoID}
		exp := r.exposures[key]
		if exp == nil {
			exp = &domainrecommendation.Exposure{
				ID:             int64(len(r.exposures) + 1),
				UserID:         w.UserID,
				VideoID:        w.VideoID,
				FirstExposedAt: now,
				LastExposedAt:  now,
				ExposureCount:  1,
				LastScene:      w.Scene,
			}
			r.exposures[key] = exp
		} else {
			exp.LastExposedAt = now
			exp.ExposureCount++
			exp.LastScene = w.Scene
		}
		copied := *exp
		res = append(res, &copied)
	}
	return res, nil
}

type memMessageEntry struct {
	msg            *domainmessage.Message
	idempotencyKey string
}

type memMessageRepo struct {
	mu     sync.Mutex
	nextID int64
	items  []*memMessageEntry
}

func newMemMessageRepo() *memMessageRepo {
	return &memMessageRepo{nextID: 1}
}

func cloneMessage(m *domainmessage.Message) *domainmessage.Message {
	if m == nil {
		return nil
	}
	copied := *m
	return &copied
}

func (r *memMessageRepo) ListByUser(_ context.Context, userID int64, cursor *domainmessage.Cursor, limit int) ([]*domainmessage.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var list []*domainmessage.Message
	for i := len(r.items) - 1; i >= 0; i-- {
		m := r.items[i].msg
		if m.UserID != userID {
			continue
		}
		if cursor != nil {
			if m.CreatedAt.After(cursor.CreatedAt) || (m.CreatedAt.Equal(cursor.CreatedAt) && m.ID >= cursor.MessageID) {
				continue
			}
		}
		list = append(list, cloneMessage(m))
		if len(list) >= limit {
			break
		}
	}
	return list, nil
}

func (r *memMessageRepo) MarkRead(_ context.Context, userID int64, messageIDs []int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idSet := map[int64]bool{}
	for _, id := range messageIDs {
		idSet[id] = true
	}
	now := time.Now().UTC()
	updated := 0
	for _, entry := range r.items {
		if entry.msg.UserID != userID || entry.msg.IsRead {
			continue
		}
		if len(idSet) > 0 && !idSet[entry.msg.ID] {
			continue
		}
		entry.msg.IsRead = true
		entry.msg.ReadAt = &now
		updated++
	}
	return updated, nil
}

func (r *memMessageRepo) CountUnread(_ context.Context, userID int64) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, entry := range r.items {
		if entry.msg.UserID == userID && !entry.msg.IsRead {
			count++
		}
	}
	return count, nil
}

func (r *memMessageRepo) Create(_ context.Context, message *domainmessage.Message, idempotencyKey string) (*domainmessage.Message, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	for _, entry := range r.items {
		if entry.msg.UserID != message.UserID {
			continue
		}
		if (message.EventID != "" && entry.msg.EventID == message.EventID) ||
			(idempotencyKey != "" && entry.idempotencyKey == idempotencyKey) {
			return cloneMessage(entry.msg), false, nil
		}
	}
	copied := cloneMessage(message)
	copied.ID = r.nextID
	r.nextID++
	copied.CreatedAt = time.Now().UTC()
	r.items = append(r.items, &memMessageEntry{
		msg:            copied,
		idempotencyKey: idempotencyKey,
	})
	return cloneMessage(copied), true, nil
}

type memPlaybackRepo struct {
	mu        sync.Mutex
	videoRepo *memVideoRepo
	configs   map[[2]string]*domainplayback.Config
	reports   []*domainplayback.QoSReport
}

func newMemPlaybackRepo(videoRepo *memVideoRepo) *memPlaybackRepo {
	return &memPlaybackRepo{
		videoRepo: videoRepo,
		configs:   map[[2]string]*domainplayback.Config{},
	}
}

func (r *memPlaybackRepo) seedConfig(platform, networkType string, preloadCount, bufferMs int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	p := domainplayback.NormalizePlatform(platform)
	n := domainplayback.NormalizeNetworkType(networkType)
	r.configs[[2]string{p, n}] = &domainplayback.Config{
		ID:           int64(len(r.configs) + 1),
		Platform:     p,
		NetworkType:  n,
		PreloadCount: preloadCount,
		BufferMs:     bufferMs,
		UpdatedAt:    time.Now().UTC(),
	}
}

func (r *memPlaybackRepo) FindConfig(_ context.Context, platform string, networkType string) (*domainplayback.Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cfg := r.configs[[2]string{platform, networkType}]
	if cfg == nil {
		return nil, nil
	}
	copied := *cfg
	return &copied, nil
}

func (r *memPlaybackRepo) ListPreloadVideos(_ context.Context, currentVideoID int64, limit int) ([]*domainplayback.PreloadVideo, error) {
	r.videoRepo.mu.Lock()
	defer r.videoRepo.mu.Unlock()
	var list []*domainplayback.PreloadVideo
	for _, v := range r.videoRepo.videos {
		if v.Status == domainvideo.StatusPublished && (currentVideoID == 0 || v.ID < currentVideoID) {
			list = append(list, domainplayback.RestorePreloadVideo(v.ID, v.MediaURL, v.CoverURL))
			if len(list) >= limit {
				break
			}
		}
	}
	return list, nil
}

func (r *memPlaybackRepo) CreateQoSReport(_ context.Context, report *domainplayback.QoSReport) (*domainplayback.QoSReport, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if report.IdempotencyKey != "" {
		for _, existing := range r.reports {
			if existing.UserID == report.UserID && existing.IdempotencyKey == report.IdempotencyKey {
				copied := *existing
				return &copied, false, nil
			}
		}
	}
	copied := *report
	copied.ID = int64(len(r.reports) + 1)
	copied.CreatedAt = time.Now().UTC()
	r.reports = append(r.reports, &copied)
	out := copied
	return &out, true, nil
}
