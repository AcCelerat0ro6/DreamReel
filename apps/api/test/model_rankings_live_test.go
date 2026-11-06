package test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	av "DreamReel/internal/application/video"
	dv "DreamReel/internal/domain/video"
	"DreamReel/internal/infra/config"
	ia "DreamReel/internal/infra/persistence/account"
	iff "DreamReel/internal/infra/persistence/feed"
	iv "DreamReel/internal/infra/persistence/video"
	mysql "github.com/go-sql-driver/mysql"
	gm "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

// Uses its own database; application data and Redis are never touched.
func TestModelRankingsLive(t *testing.T) {
	if os.Getenv("DREAMREEL_MODEL_LIVE") != "1" {
		t.Skip("set DREAMREEL_MODEL_LIVE=1 for isolated MySQL integration test")
	}
	cfg, err := config.LoadConfig("../configs/config.yaml")
	if err != nil {
		t.Fatal(err)
	}
	mc := mysql.NewConfig()
	mc.User, mc.Passwd, mc.Net = cfg.Database.User, cfg.Database.Password, "tcp"
	mc.Addr, mc.ParseTime, mc.Timeout = fmt.Sprintf("%s:%d", cfg.Database.Host, cfg.Database.Port), true, 5*time.Second
	admin, err := sql.Open("mysql", mc.FormatDSN())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = admin.Close() })
	dbName := fmt.Sprintf("dreamreel_model_test_%d", time.Now().UnixNano())
	if _, err := admin.Exec("CREATE DATABASE `" + dbName + "` CHARACTER SET utf8mb4"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if strings.HasPrefix(dbName, "dreamreel_model_test_") {
			if _, err := admin.Exec("DROP DATABASE `" + dbName + "`"); err != nil {
				t.Error(err)
			}
		}
	})
	mc.DBName = dbName
	db, err := gorm.Open(gm.Open(mc.FormatDSN()), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		t.Fatal(err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	if err := db.AutoMigrate(&iv.VideoModel{}, &iv.VideoStatModel{}, &ia.UserModel{}); err != nil {
		t.Fatal(err)
	}
	repo := iv.New(db)
	ctx := context.Background()
	service := av.NewRanking(repo)
	empty, err := service.List(ctx, "hot", 20, 0)
	if err != nil || len(empty.Items) != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
	seed := func(model string, status, likes, comments, favorites int) *dv.Video {
		t.Helper()
		params, style, origin := "seed=42", "水墨", int64(12)
		v, err := dv.NewPublished(1, "Test", "", "/test.mp4", "/test.jpg", &model, &params, &style, &origin, "")
		if err != nil {
			t.Fatal(err)
		}
		v.Status, v.LikeCount, v.CommentCount, v.FavoriteCount = status, likes, comments, favorites
		if err := repo.Save(ctx, v); err != nil {
			t.Fatal(err)
		}
		return v
	}
	a := seed(" Atlas ", dv.StatusPublished, 2, 1, 0)
	seed("atlas", dv.StatusPublished, 1, 0, 1)  // same model, score = 18
	seed("Boreal", dv.StatusPublished, 0, 4, 0) // score = 20
	seed("Cinder", dv.StatusPublished, 4, 0, 0)
	seed("Delta", dv.StatusPublished, 4, 0, 0) // stable name tie-break
	missingStat := seed("EmptyStats", dv.StatusPublished, 0, 0, 0)
	if err := db.Delete(&iv.VideoStatModel{}, "video_id = ?", missingStat.ID).Error; err != nil {
		t.Fatal(err)
	}
	seed("", dv.StatusPublished, 100, 100, 100)
	seed("Hidden", dv.StatusDeleted, 100, 100, 100)
	seed("Hidden", dv.StatusOffline, 100, 100, 100)
	seed("Hidden", dv.StatusDraft, 100, 100, 100)
	noDate := seed("Hidden", dv.StatusPublished, 100, 100, 100)
	if err := db.Model(&iv.VideoModel{}).Where("id = ?", noDate.ID).Update("published_at", nil).Error; err != nil {
		t.Fatal(err)
	}
	page, err := service.List(ctx, "hot", 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !page.HasMore || len(page.Items) != 2 || page.Items[0].ModelName != "Boreal" || page.Items[1].VideoCount != 2 || page.Items[1].HotScore != 18 {
		t.Fatalf("hot: %+v", page)
	}
	page, err = service.List(ctx, "hot", 2, 2)
	if err != nil || len(page.Items) != 2 || page.Items[0].ModelName != "Cinder" || page.Items[1].ModelName != "Delta" {
		t.Fatalf("ties/page: %+v %v", page, err)
	}
	page, err = service.List(ctx, "works", 20, 0)
	if err != nil || len(page.Items) != 5 || page.Items[0].VideoCount != 2 || page.HasMore {
		t.Fatalf("works: %+v %v", page, err)
	}
	page, err = service.List(ctx, "likes", 20, 0)
	if err != nil || page.Items[0].ModelName != "Cinder" || page.Items[4].ModelName != "EmptyStats" {
		t.Fatalf("likes: %+v %v", page, err)
	}
	cards, err := iff.New(db).BatchGetFeedCards(ctx, []int64{a.ID})
	if err != nil {
		t.Fatal(err)
	}
	card := cards[a.ID]
	if card == nil || card.ModelParams == nil || *card.ModelParams != "seed=42" || card.AIStyleTag == nil || *card.AIStyleTag != "水墨" || card.OriginVideoID == nil || *card.OriginVideoID != 12 {
		t.Fatalf("feed metadata: %+v", card)
	}
	loaded, err := repo.FindByID(ctx, a.ID)
	if err != nil || loaded.ModelName == nil || *loaded.ModelName != "Atlas" {
		t.Fatalf("persisted metadata: %+v %v", loaded, err)
	}
	if err := db.Model(&iv.VideoModel{}).Where("id = ?", a.ID).Update("status", dv.StatusDeleted).Error; err != nil {
		t.Fatal(err)
	}
	page, err = service.List(ctx, "hot", 20, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range page.Items {
		if strings.EqualFold(item.ModelName, "Atlas") && (item.VideoCount != 1 || item.HotScore != 7) {
			t.Fatalf("deleted work still counted: %+v", item)
		}
	}
}
