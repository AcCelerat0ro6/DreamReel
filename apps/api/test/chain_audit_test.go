package test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	ae "DreamReel/internal/application/embedding"
	af "DreamReel/internal/application/feed"
	ai "DreamReel/internal/application/interaction"
	ar "DreamReel/internal/application/recommendation"
	av "DreamReel/internal/application/video"
	df "DreamReel/internal/domain/feed"
	"DreamReel/internal/infra/cache"
	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/mq"
	ie "DreamReel/internal/infra/persistence/embedding"
	iff "DreamReel/internal/infra/persistence/feed"
	ii "DreamReel/internal/infra/persistence/interaction"
	ir "DreamReel/internal/infra/persistence/recommendation"
	iv "DreamReel/internal/infra/persistence/video"
	"DreamReel/internal/interfaces/http/middleware"
	"DreamReel/internal/interfaces/http/router"
	upload "DreamReel/internal/interfaces/http/upload"
	"github.com/gin-gonic/gin"
	mysql "github.com/go-sql-driver/mysql"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	gm "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Opt-in audit: uses an independently created MySQL database, an empty Redis DB,
// and uniquely named RabbitMQ resources. Never enable against shared production infrastructure.
func TestAuditLiveChains(t *testing.T) {
	if os.Getenv("DREAMREEL_AUDIT_LIVE") != "1" {
		t.Skip("set DREAMREEL_AUDIT_LIVE=1 for isolated local integration audit")
	}
	cfg, err := config.LoadConfig("../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	mc := mysql.NewConfig()
	mc.User = cfg.Database.User
	mc.Passwd = cfg.Database.Password
	mc.Net = "tcp"
	mc.Addr = fmt.Sprintf("%s:%d", cfg.Database.Host, cfg.Database.Port)
	mc.ParseTime = true
	admin, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	pingctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
	err = admin.PingContext(pingctx)
	pcancel()
	if err != nil {
		t.Fatalf("mysql connect: %v", err)
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	dbName := "dreamreel_audit_" + suffix
	if _, err = admin.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if strings.HasPrefix(dbName, "dreamreel_audit_") {
			if _, err := admin.Exec("DROP DATABASE `" + dbName + "`"); err != nil {
				t.Errorf("audit DB cleanup: %v", err)
			}
		}
	})
	mc.DBName = dbName
	db, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	// Reserve an empty database, instead of clearing or reusing application Redis data.
	var rdb *redis.Client
	for n := 15; n >= 1; n-- {
		r := redis.NewClient(&redis.Options{Addr: cfg.Redis.Addr, Password: cfg.Redis.Password, DB: n, DialTimeout: time.Second})
		size, e := r.DBSize(ctx).Result()
		if e == nil && size == 0 {
			ok, e := r.SetNX(ctx, "dreamreel:audit:reservation", suffix, 10*time.Minute).Result()
			if e == nil && ok {
				rdb = r
				break
			}
		}
		_ = r.Close()
	}
	if rdb == nil {
		t.Fatal("no empty Redis DB available for audit")
	}
	t.Cleanup(func() {
		keys, e := rdb.Keys(context.Background(), "*").Result()
		if e != nil {
			t.Errorf("audit redis cleanup: %v", e)
		} else if len(keys) > 0 {
			if e := rdb.Del(context.Background(), keys...).Err(); e != nil {
				t.Errorf("audit redis cleanup: %v", e)
			}
		}
		_ = rdb.Close()
	})
	prefix := "dreamreel.audit." + suffix
	cfg.RabbitMQ.InteractionExchange = prefix + ".interaction"
	cfg.RabbitMQ.ActionChangedQueue = prefix + ".action"
	cfg.RabbitMQ.ActionChangedRouting = "action"
	cfg.RabbitMQ.VideoExchange = prefix + ".video"
	cfg.RabbitMQ.VideoPublishedQueue = prefix + ".fanout"
	cfg.RabbitMQ.VideoEmbeddingQueue = prefix + ".embedding"
	cfg.RabbitMQ.VideoPublishedRouting = "published"
	cfg.RabbitMQ.ExposureExchange = prefix + ".exposure"
	cfg.RabbitMQ.ViewEventRecordedQueue = prefix + ".view"
	cfg.RabbitMQ.ViewEventRecordedRouting = "view"
	broker, err := mq.NewRabbitMQ(cfg.RabbitMQ)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cancel()
		_ = broker.Close()
		conn, e := amqp.Dial(cfg.RabbitMQ.URL)
		if e != nil {
			t.Errorf("audit rabbit cleanup: %v", e)
			return
		}
		defer conn.Close()
		ch, e := conn.Channel()
		if e != nil {
			t.Error(e)
			return
		}
		defer ch.Close()
		for _, q := range []string{cfg.RabbitMQ.ActionChangedQueue, cfg.RabbitMQ.VideoPublishedQueue, cfg.RabbitMQ.VideoEmbeddingQueue, cfg.RabbitMQ.ViewEventRecordedQueue} {
			if strings.HasPrefix(q, prefix) {
				if _, e = ch.QueueDelete(q, false, false, false); e != nil {
					t.Error(e)
				}
			}
		}
		for _, x := range []string{cfg.RabbitMQ.InteractionExchange, cfg.RabbitMQ.VideoExchange, cfg.RabbitMQ.ExposureExchange} {
			if strings.HasPrefix(x, prefix) {
				if e = ch.ExchangeDelete(x, false, false); e != nil {
					t.Error(e)
				}
			}
		}
	})
	gin.SetMode(gin.TestMode)
	g := gin.New()
	g.Use(gin.Recovery())
	cfg.JWT.Secret = "audit-jwt-" + suffix
	cfg.Internal.Token = "audit-internal-" + suffix
	if err := router.Register(g, cfg, db, rdb, broker); err != nil {
		t.Fatalf("production router/migration: %v", err)
	}
	gd, err := gorm.Open(gm.New(gm.Config{Conn: db}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	fc := cache.NewFeedCache(rdb)
	fr := iff.New(gd)
	if err := ai.NewActionWorker(ii.New(gd), broker).Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := av.NewFanoutWorker(fr, broker, fc, av.NewFeedPreheater(fr, fc)).Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ae.NewVideoEmbeddingWorker(ae.New(ie.New(gd), nil), broker).Start(ctx); err != nil {
		t.Fatal(err)
	}
	if err := ar.NewProfileWorker(ar.New(ir.New(gd)), broker).Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Logf("isolated real infrastructure: MySQL=%s RedisDB=%d RabbitMQ=%s", dbName, rdb.Options().DB, prefix)
	request := func(t *testing.T, method, path, tok string, body any, want int) map[string]any {
		t.Helper()
		headers := map[string]string{}
		if tok != "" {
			headers["Authorization"] = "Bearer " + tok
		}
		r := doJSONRequest(t, g, method, path, body, headers)
		if r.Code != want {
			t.Fatalf("%s %s: HTTP %d want %d: %s", method, path, r.Code, want, r.Body.String())
		}
		var out map[string]any
		if r.Body.Len() > 0 {
			if e := json.Unmarshal(r.Body.Bytes(), &out); e != nil {
				t.Fatal(e)
			}
		}
		return out
	}
	user := func(name string) (int64, string) {
		r := request(t, "POST", "/api/users", "", map[string]any{"account": name, "password": "audit-pass-123456", "nickname": name}, 201)
		id := int64(r["id"].(float64))
		login := request(t, "POST", "/api/sessions", "", map[string]any{"account": name, "password": "audit-pass-123456"}, 200)
		return id, login["access_token"].(string)
	}
	author, atok := user("audit_author")
	viewer, vtok := user("audit_viewer")
	create := func(t *testing.T) int64 {
		r := request(t, "POST", "/api/videos", atok, map[string]any{"title": "audit video", "description": "audit content", "media_url": "/uploads/video/audit.mp4", "cover_url": "/uploads/cover/audit.png"}, 201)
		return int64(r["id"].(float64))
	}
	wait := func(t *testing.T, label string, fn func() bool) {
		t.Helper()
		end := time.Now().Add(5 * time.Second)
		for time.Now().Before(end) {
			if fn() {
				return
			}
			time.Sleep(25 * time.Millisecond)
		}
		t.Fatalf("timed out: %s", label)
	}
	var videoID int64
	t.Run("01_smoke_registration_follow_publish_workers", func(t *testing.T) {
		request(t, "GET", "/api/users/me", vtok, nil, 200)
		request(t, "PATCH", "/api/users/me", vtok, map[string]any{"bio": "audit bio"}, 200)
		request(t, "PUT", fmt.Sprintf("/api/users/me/following/%d", author), vtok, nil, 200)
		videoID = create(t)
		wait(t, "embedding and following fanout", func() bool {
			var n int
			_ = db.QueryRow("SELECT COUNT(*) FROM video_embedding WHERE video_id=?", videoID).Scan(&n)
			z, _ := rdb.ZCard(ctx, fmt.Sprintf("feed:following:inbox:v1:%d", viewer)).Result()
			return n == 1 && z == 1
		})
		request(t, "GET", fmt.Sprintf("/api/videos/%d", videoID), "", nil, 200)
		for _, scene := range []string{"timeline", "following", "hot", "recommend"} {
			request(t, "POST", "/api/feed-queries", vtok, map[string]any{"scene": scene, "limit": 10}, 200)
		}
	})
	if videoID == 0 {
		t.Fatal("smoke fixture was not created")
	}
	t.Run("02_smoke_interactions_messages_exposure_playback", func(t *testing.T) {
		for _, action := range []string{"like", "favorite"} {
			request(t, "PUT", fmt.Sprintf("/api/videos/%d/%s", videoID, action), vtok, nil, 200)
		}
		wait(t, "async action persistence", func() bool {
			var l, f int
			_ = db.QueryRow("SELECT like_count,favorite_count FROM video_stat WHERE video_id=?", videoID).Scan(&l, &f)
			return l == 1 && f == 1
		})
		request(t, "POST", fmt.Sprintf("/api/videos/%d/comments", videoID), vtok, map[string]any{"content": "audit comment"}, 201)
		request(t, "GET", fmt.Sprintf("/api/videos/%d/comments", videoID), "", nil, 200)
		request(t, "GET", "/api/messages", atok, nil, 200)
		request(t, "GET", "/api/message-stats/unread", atok, nil, 200)
		request(t, "POST", "/api/video-view-events", vtok, map[string]any{"video_id": videoID, "scene": "recommend", "event_type": "complete", "watch_ms": 30000, "completed": true}, 201)
		wait(t, "interest profile", func() bool {
			var n int
			_ = db.QueryRow("SELECT COUNT(*) FROM user_interest_profile WHERE user_id=?", viewer).Scan(&n)
			return n == 1
		})
		request(t, "GET", "/api/playback-config", vtok, nil, 200)
		request(t, "GET", "/api/preload-videos", vtok, nil, 200)
		request(t, "POST", "/api/playback-qos-reports", vtok, map[string]any{"video_id": videoID, "first_frame_ms": 100, "watch_ms": 30000}, 201)
	})
	t.Run("03_internal_routes_require_auth", func(t *testing.T) {
		for _, p := range []string{"recommendation-candidates", "exposure-decisions", "exposures"} {
			r := doJSONRequest(t, g, "POST", "/internal/"+p, map[string]any{"user_id": viewer, "scene": "recommend", "video_ids": []int64{videoID}}, nil)
			if r.Code != 401 {
				t.Errorf("unauthenticated /internal/%s: HTTP %d", p, r.Code)
			}
		}
	})
	t.Run("04_comment_count_survives_json_ttl", func(t *testing.T) {
		if err := rdb.Del(ctx, fmt.Sprintf("video:stat:v1:%d", videoID)).Err(); err != nil {
			t.Fatal(err)
		}
		stats, err := fc.GetStats(ctx, []int64{videoID})
		if err != nil {
			t.Fatal(err)
		}
		var n int
		_ = db.QueryRow("SELECT comment_count FROM video_stat WHERE video_id=?", videoID).Scan(&n)
		if stats[videoID] == nil || stats[videoID].CommentCount != n {
			t.Errorf("comment cache stale: Redis=%+v MySQL=%d", stats[videoID], n)
		}
	})
	t.Run("05_cold_action_cache_unlike_reaches_mysql", func(t *testing.T) {
		key := fmt.Sprintf("interaction:action:v1:%d:%d:like", viewer, videoID)
		if err := rdb.Del(ctx, key).Err(); err != nil {
			t.Fatal(err)
		}
		request(t, "DELETE", fmt.Sprintf("/api/videos/%d/like", videoID), vtok, nil, 200)
		time.Sleep(300 * time.Millisecond)
		var status, count int
		_ = db.QueryRow("SELECT status FROM interaction_action WHERE user_id=? AND video_id=? AND action_type='LIKE'", viewer, videoID).Scan(&status)
		_ = db.QueryRow("SELECT like_count FROM video_stat WHERE video_id=?", videoID).Scan(&count)
		if status != 2 || count != 0 {
			t.Errorf("unlike HTTP 200 but MySQL remains active: status=%d like_count=%d", status, count)
		}
	})
	t.Run("06_unfollow_removes_old_inbox_items", func(t *testing.T) {
		request(t, "DELETE", fmt.Sprintf("/api/users/me/following/%d", author), vtok, nil, 200)
		r := request(t, "POST", "/api/feed-queries", vtok, map[string]any{"scene": "following", "limit": 10}, 200)
		items, _ := r["items"].([]any)
		if len(items) > 0 {
			t.Errorf("unfollowed author still in following feed: items=%d", len(items))
		}
	})
	t.Run("07_deleted_video_not_served_from_cache", func(t *testing.T) {
		request(t, "POST", "/api/feed-queries", vtok, map[string]any{"scene": "timeline", "limit": 10}, 200)
		request(t, "DELETE", fmt.Sprintf("/api/videos/%d", videoID), atok, nil, 204)
		request(t, "GET", fmt.Sprintf("/api/videos/%d", videoID), "", nil, 404)
		r := request(t, "POST", "/api/feed-queries", vtok, map[string]any{"scene": "timeline", "limit": 10}, 200)
		items, _ := r["items"].([]any)
		if len(items) > 0 {
			t.Errorf("deleted video still in cached timeline: items=%d", len(items))
		}
	})
	t.Run("08_logout_revokes_old_token", func(t *testing.T) {
		request(t, "DELETE", "/api/sessions/current", vtok, nil, 204)
		r := doJSONRequest(t, g, "GET", "/api/users/me", nil, map[string]string{"Authorization": "Bearer " + vtok})
		if r.Code != 401 {
			t.Errorf("logout leaves JWT usable: HTTP %d", r.Code)
		}
	})
	t.Run("09_disabled_account_cannot_publish", func(t *testing.T) {
		if _, err := db.Exec("UPDATE account SET status=2 WHERE id=?", author); err != nil {
			t.Fatal(err)
		}
		defer db.Exec("UPDATE account SET status=1 WHERE id=?", author)
		r := doJSONRequest(t, g, "POST", "/api/videos", map[string]any{"title": "audit", "media_url": "/a.mp4", "cover_url": "/a.png"}, map[string]string{"Authorization": "Bearer " + atok})
		if r.Code != 401 && r.Code != 403 {
			t.Errorf("disabled account can publish: HTTP %d", r.Code)
		}
	})
	t.Run("10_html_upload_is_not_executable_on_app_origin", func(t *testing.T) {
		root := t.TempDir()
		u := gin.New()
		u.POST("/api/uploads", func(c *gin.Context) { c.Set(middleware.ContextUserIDKey, viewer) }, upload.New(root).Create)
		u.Static("/uploads", root)
		var b bytes.Buffer
		w := multipart.NewWriter(&b)
		part, _ := w.CreateFormFile("file", "audit.html")
		_, _ = part.Write([]byte("<!doctype html><html><body>audit</body></html>"))
		_ = w.Close()
		req := httptest.NewRequest("POST", "/api/uploads", &b)
		req.Header.Set("Content-Type", w.FormDataContentType())
		rec := httptest.NewRecorder()
		u.ServeHTTP(rec, req)
		if rec.Code == 201 {
			var out struct {
				URL string `json:"url"`
			}
			_ = json.Unmarshal(rec.Body.Bytes(), &out)
			get := httptest.NewRecorder()
			u.ServeHTTP(get, httptest.NewRequest("GET", out.URL, nil))
			if strings.Contains(get.Header().Get("Content-Type"), "text/html") && get.Header().Get("Content-Disposition") == "" {
				t.Errorf("HTML upload is publicly served inline: HTTP %d content-type=%s", get.Code, get.Header().Get("Content-Type"))
			}
		}
	})
	t.Run("11_following_and_hot_fall_back_on_redis_failure", func(t *testing.T) {
		bad := redis.NewClient(&redis.Options{Addr: "127.0.0.1:1", DialTimeout: 20 * time.Millisecond, MaxRetries: -1})
		defer bad.Close()
		s := af.New(fr, af.WithFeedCache(cache.NewFeedCache(bad)))
		for _, scene := range []df.Scene{df.SceneFollowing, df.SceneHot} {
			if _, err := s.GetFeed(ctx, af.FeedRequest{Scene: scene, ViewerID: viewer, Limit: 10}); err != nil {
				t.Errorf("%s returns error instead of DB fallback: %v", scene, err)
			}
		}
	})
	t.Run("12_publish_failure_is_repaired_on_idempotent_retry", func(t *testing.T) {
		closed, e := mq.NewRabbitMQ(cfg.RabbitMQ)
		if e != nil {
			t.Fatal(e)
		}
		_ = closed.Close()
		s := av.New(iv.New(gd), av.WithPublishedEventPublisher(closed))
		first, e := s.CreatePublished(ctx, author, "lost event", "audit", "/a.mp4", "/a.png", nil, nil, nil, nil, "audit-lost-event")
		if e != nil {
			t.Fatal(e)
		}
		second, e := s.CreatePublished(ctx, author, "lost event", "audit", "/a.mp4", "/a.png", nil, nil, nil, nil, "audit-lost-event")
		if e != nil {
			t.Fatal(e)
		}
		time.Sleep(200 * time.Millisecond)
		var n int
		_ = db.QueryRow("SELECT COUNT(*) FROM video_embedding WHERE video_id=?", first.Video.ID).Scan(&n)
		if n == 0 {
			t.Errorf("DB saved video and both calls succeeded, but publication event was lost: first_created=%v retry_created=%v embeddings=%d", first.Created, second.Created, n)
		}
	})
	t.Run("13_recommendation_pages_beyond_initial_pool", func(t *testing.T) {
		checkLiveRecommendationSession(t, g, gd, rdb, author, atok, cfg.Internal.Token)
	})
	t.Run("14_out_of_order_action_events_keep_latest_state", func(t *testing.T) {
		r, e := av.New(iv.New(gd)).CreatePublished(ctx, author, "order audit", "audit", "/a.mp4", "/a.png", nil, nil, nil, nil, "")
		if e != nil {
			t.Fatal(e)
		}
		w := ai.NewActionWorker(ii.New(gd), nil)
		older := ai.NewActionChangedEvent(viewer, r.Video.ID, "LIKE", true, "audit-old")
		newer := ai.NewActionChangedEvent(viewer, r.Video.ID, "LIKE", false, "audit-new")
		newer.OccurredAt = older.OccurredAt.Add(time.Second)
		if e = w.HandleActionChanged(ctx, newer); e != nil {
			t.Fatal(e)
		}
		if e = w.HandleActionChanged(ctx, older); e != nil {
			t.Fatal(e)
		}
		var status int
		_ = db.QueryRow("SELECT status FROM interaction_action WHERE user_id=? AND video_id=? AND action_type='LIKE'", viewer, r.Video.ID).Scan(&status)
		if status != 2 {
			t.Errorf("older event overwrites newer unlike: status=%d", status)
		}
	})
}
