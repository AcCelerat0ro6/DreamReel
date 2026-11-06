package test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"

	app "DreamReel/internal/application/recommendation"
	domain "DreamReel/internal/domain/recommendation"
	domainvideo "DreamReel/internal/domain/video"
	infravideo "DreamReel/internal/infra/persistence/video"
	"github.com/redis/go-redis/v9"
	"gorm.io/gorm"
)

type failingRecallRepo struct {
	*memRecommendationRepo
	fail bool
}

func (r *failingRecallRepo) ListCandidatePool(ctx context.Context, userID int64, excludeIDs []int64, limit int) ([]*domain.Candidate, error) {
	if r.fail {
		return nil, errors.New("recall unavailable")
	}
	return r.memRecommendationRepo.ListCandidatePool(ctx, userID, excludeIDs, limit)
}

// TestRecommendationRefillRetry 验证补池失败后的重试沿用原队列位置。
func TestRecommendationRefillRetry(t *testing.T) {
	repo := &failingRecallRepo{memRecommendationRepo: newMemRecommendationRepo(newMemVideoRepo())}
	for i := 0; i < 251; i++ {
		repo.videoRepo.seedVideo(1, "分页测试", time.Now(), domainvideo.StatusPublished, 0, 0, 0)
	}
	svc := app.New(repo)
	ctx := context.Background()
	req := app.CandidateRequest{UserID: 1, Scene: "recommend", Limit: 100}
	first, err := svc.Recommend(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if !first.HasMore || len(first.Candidates) != 100 {
		t.Fatal("首批分页错误")
	}
	req.Cursor = first.NextCursor
	repo.fail = true
	if _, err := svc.Recommend(ctx, req); !errors.Is(err, app.ErrLoadRecommendationFailed) {
		t.Fatalf("补池失败应返回可重试错误: %v", err)
	}
	repo.fail = false
	second, err := svc.Recommend(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	replay, err := svc.Recommend(ctx, req)
	if err != nil || !reflect.DeepEqual(second, replay) {
		t.Fatalf("重试返回了不同页面: %v", err)
	}
	req.Cursor = second.NextCursor
	last, err := svc.Recommend(ctx, req)
	if err != nil {
		t.Fatal(err)
	}
	if len(last.Candidates) != 51 || last.HasMore || last.NextCursor != "" {
		t.Fatal("尾页数量或结束标记错误")
	}
}

// checkLiveRecommendationSession 通过生产路由跨越两次补池，校验 Redis 提交、下架过滤和过期响应。
func checkLiveRecommendationSession(t *testing.T, engine http.Handler, db *gorm.DB, rdb *redis.Client, author int64, token, internalToken string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now()
	videos := make([]infravideo.VideoModel, 450)
	for i := range videos {
		videos[i] = infravideo.VideoModel{AuthorID: author, Title: fmt.Sprintf("session-%03d", i), MediaURL: "/a.mp4", CoverURL: "/a.png", Status: domainvideo.StatusPublished, PublishedAt: &now}
	}
	if err := db.CreateInBatches(videos, 100).Error; err != nil {
		t.Fatal(err)
	}
	var total int64
	if err := db.Model(&infravideo.VideoModel{}).Where("status = ?", domainvideo.StatusPublished).Count(&total).Error; err != nil {
		t.Fatal(err)
	}

	headers := map[string]string{"X-Internal-Token": internalToken}
	query := func(userID int64, scene, cursor string) *httptest.ResponseRecorder {
		return doJSONRequest(t, engine, http.MethodPost, "/internal/recommendation-candidates", map[string]any{"user_id": userID, "scene": scene, "limit": 100, "cursor": cursor}, headers)
	}
	type page struct {
		Candidates []struct {
			VideoID int64 `json:"video_id"`
		} `json:"candidates"`
		Next string `json:"next_cursor"`
		More bool   `json:"has_more"`
	}
	decode := func(rec *httptest.ResponseRecorder) page {
		t.Helper()
		var p page
		if rec.Code != 200 {
			t.Fatalf("获取推荐页失败: %d %s", rec.Code, rec.Body.String())
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	first := decode(query(author, "recommend", ""))
	if len(first.Candidates) != 100 || !first.More {
		t.Fatal("首批分页错误")
	}
	raw, err := base64.RawURLEncoding.DecodeString(first.Next)
	if err != nil {
		t.Fatal(err)
	}
	var cursor domain.Cursor
	if err := json.Unmarshal(raw, &cursor); err != nil || !cursor.Valid() {
		t.Fatalf("无效会话游标: %v", err)
	}
	prefix := "rec:session:{" + cursor.SessionID + "}"
	if n, err := rdb.LLen(ctx, prefix+":items").Result(); err != nil || n != 200 {
		t.Fatalf("首次召回应写入 200 条: n=%d err=%v", n, err)
	}
	if query(author+1, "recommend", first.Next).Code != 400 || query(author, "other", first.Next).Code != 400 {
		t.Fatal("游标被其他用户或场景复用")
	}

	// 同一游标触发并发补池，所有请求都应读到同一页。
	replies := make(chan *httptest.ResponseRecorder, 8)
	start := make(chan struct{})
	for range cap(replies) {
		go func() { <-start; replies <- query(author, "recommend", first.Next) }()
	}
	close(start)
	second := decode(<-replies)
	for i := 1; i < cap(replies); i++ {
		other := decode(<-replies)
		if !reflect.DeepEqual(second, other) {
			t.Fatal("并发翻页顺序不一致")
		}
	}
	if n, err := rdb.LLen(ctx, prefix+":items").Result(); err != nil || n != 400 {
		t.Fatalf("并发补池应只提交一批: n=%d err=%v", n, err)
	}

	// 删除第三页开头的候选，队列中的原始位置仍需保留。
	entry, err := rdb.LIndex(ctx, prefix+":items", 200).Bytes()
	if err != nil {
		t.Fatal(err)
	}
	var removed domain.Candidate
	if err := json.Unmarshal(entry, &removed); err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&infravideo.VideoModel{}).Where("id = ?", removed.VideoID).Update("status", domainvideo.StatusDeleted).Error; err != nil {
		t.Fatal(err)
	}
	seen := map[int64]bool{}
	collect := func(p page) {
		t.Helper()
		if p.More && len(p.Candidates) != 100 {
			t.Fatal("中间页未补满")
		}
		for _, v := range p.Candidates {
			if seen[v.VideoID] || v.VideoID == removed.VideoID {
				t.Fatalf("重复或已删除的视频: %d", v.VideoID)
			}
			seen[v.VideoID] = true
		}
	}
	collect(first)
	collect(second)
	for p := second; p.More; {
		if p.Next == "" {
			t.Fatal("缺少下一页游标")
		}
		p = decode(query(author, "recommend", p.Next))
		collect(p)
		if !p.More && p.Next != "" {
			t.Fatal("尾页仍返回了游标")
		}
	}
	if int64(len(seen)) != total-1 {
		t.Fatalf("跨批漏项: 得到 %d 条，应有 %d 条", len(seen), total-1)
	}

	// 模拟会话到期，从内部接口和公开 Feed 入口都应得到明确的刷新信号。
	if err := rdb.Expire(ctx, prefix+":meta", 0).Err(); err != nil {
		t.Fatal(err)
	}
	expired := query(author, "recommend", first.Next)
	feed := doJSONRequest(t, engine, http.MethodPost, "/api/feed-queries", map[string]any{"scene": "recommend", "cursor": first.Next}, map[string]string{"Authorization": "Bearer " + token})
	for _, rec := range []*httptest.ResponseRecorder{expired, feed} {
		var body struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || rec.Code != 410 || body.Error != "cursor_expired" {
			t.Fatalf("过期响应错误: %d %s", rec.Code, rec.Body.String())
		}
	}
	t.Logf("跨批返回 %d 条可见视频，并发补池、同游标重试、删除过滤及过期响应通过", len(seen))
}
