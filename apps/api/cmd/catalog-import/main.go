// catalog-import imports verified local media with traceable synthetic interactions.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	mathrand "math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	embedding "DreamReel/internal/domain/embedding"
	"DreamReel/internal/infra/cache"
	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/database"
	account "DreamReel/internal/infra/persistence/account"
	embeddingmodel "DreamReel/internal/infra/persistence/embedding"
	feed "DreamReel/internal/infra/persistence/feed"
	interaction "DreamReel/internal/infra/persistence/interaction"
	relation "DreamReel/internal/infra/persistence/relation"
	video "DreamReel/internal/infra/persistence/video"
	"DreamReel/internal/infra/redis"

	gormmysql "gorm.io/driver/mysql"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

type entry struct {
	BVID         string          `json:"bvid"`
	Title        string          `json:"title"`
	SourceURL    string          `json:"source_url"`
	SourceAuthor string          `json:"source_author"`
	SourceStats  json.RawMessage `json:"source_stats"`
	Category     string          `json:"category"`
	ModelName    string          `json:"model_name"`
	MediaURL     string          `json:"media_url"`
	CoverURL     string          `json:"cover_url"`
}
type manifest struct {
	Batch         string  `json:"batch"`
	Authorization string  `json:"authorization"`
	Videos        []entry `json:"videos"`
}
type importBatch struct {
	ID                  string `gorm:"primaryKey;size:64"`
	ManifestSHA256      string `gorm:"size:64"`
	SyntheticEngagement bool
	Accounts            int
	Videos              int
	Likes               int
	Favorites           int
	Comments            int
	Follows             int
	CreatedAt           time.Time
}

func (importBatch) TableName() string { return "catalog_import_batch" }

type importSource struct {
	VideoID      int64  `gorm:"primaryKey;autoIncrement:false"`
	BatchID      string `gorm:"size:64;index"`
	BVID         string `gorm:"size:16;uniqueIndex"`
	SourceURL    string `gorm:"size:512"`
	SourceAuthor string `gorm:"size:128"`
	SourceStats  string `gorm:"type:json"`
}

func (importSource) TableName() string { return "catalog_import_source" }

type importAccount struct {
	UserID  int64  `gorm:"primaryKey;autoIncrement:false"`
	BatchID string `gorm:"size:64;index"`
}

func (importAccount) TableName() string { return "catalog_import_account" }

var bvidPattern = regexp.MustCompile(`^BV[0-9A-Za-z]{10}$`)
var batchPattern = regexp.MustCompile(`^[a-z0-9-]{1,64}$`)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run() error {
	configPath := flag.String("config", "", "server configuration")
	manifestPath := flag.String("manifest", "", "verified ready.json")
	root := flag.String("root", "", "absolute project root")
	apply := flag.Bool("apply", false, "commit database import; otherwise preflight only")
	flag.Parse()
	content, err := os.ReadFile(*manifestPath)
	if err != nil {
		return err
	}
	var data manifest
	if err = json.Unmarshal(content, &data); err != nil {
		return err
	}
	if !batchPattern.MatchString(data.Batch) || data.Authorization != "operator-confirmed-in-chat" || len(data.Videos) < 30 {
		return errors.New("invalid or unapproved import manifest; need 30 verified videos")
	}
	seen := map[string]bool{}
	for _, e := range data.Videos {
		if !bvidPattern.MatchString(e.BVID) || seen[e.BVID] || e.SourceURL != "https://www.bilibili.com/video/"+e.BVID+"/" || len([]rune(e.Title)) > 128 || e.Title == "" {
			return fmt.Errorf("invalid source: %s", e.BVID)
		}
		seen[e.BVID] = true
		for _, url := range []string{e.MediaURL, e.CoverURL} {
			if !strings.HasPrefix(url, "/uploads/catalog/") || strings.Contains(url, "..") {
				return errors.New("media must be local verified catalog assets")
			}
			stat, err := os.Stat(filepath.Join(*root, "apps/api", filepath.FromSlash(url[1:])))
			if err != nil || stat.Size() == 0 {
				return fmt.Errorf("missing media: %s", url)
			}
		}
	}
	fmt.Printf("Preflight passed: %d local videos and covers; synthetic interactions explicitly tracked.\n", len(data.Videos))
	if !*apply {
		return nil
	}
	cfg, err := config.LoadConfig(*configPath)
	if err != nil {
		return err
	}
	db, err := database.New(&cfg.Database)
	if err != nil {
		return err
	}
	defer db.Close()
	gdb, err := gorm.Open(gormmysql.New(gormmysql.Config{Conn: db}), &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)})
	if err != nil {
		return err
	}
	if err = gdb.AutoMigrate(&importBatch{}, &importSource{}, &importAccount{}); err != nil {
		return err
	}
	lockConn, err := db.Conn(context.Background())
	if err != nil {
		return err
	}
	defer lockConn.Close()
	var locked int
	if err = lockConn.QueryRowContext(context.Background(), "SELECT GET_LOCK(?, 0)", "dreamreel-catalog-import").Scan(&locked); err != nil || locked != 1 {
		return errors.New("another import is running")
	}
	defer lockConn.ExecContext(context.Background(), "SELECT RELEASE_LOCK(?)", "dreamreel-catalog-import")
	var previous importBatch
	err = gdb.First(&previous, "id = ?", data.Batch).Error
	if err == nil {
		fmt.Printf("Batch already imported; no changes: %+v\n", previous)
		return nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	backupDir := filepath.Join(*root, ".runtime", "backups")
	if err = os.MkdirAll(backupDir, 0700); err != nil {
		return err
	}
	backupPath := filepath.Join(backupDir, data.Batch+"-before-"+time.Now().UTC().Format("20060102T150405Z")+".sql")
	backup, err := os.OpenFile(backupPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	command := exec.Command("mysqldump", "--single-transaction", "--no-tablespaces", "--set-gtid-purged=OFF", "-h", cfg.Database.Host, "-P", fmt.Sprint(cfg.Database.Port), "-u", cfg.Database.User, cfg.Database.Name)
	command.Env = append(os.Environ(), "MYSQL_PWD="+cfg.Database.Password)
	command.Stdout = backup
	err = command.Run()
	closeErr := backup.Close()
	if err != nil {
		return fmt.Errorf("database backup failed: %w", err)
	}
	if closeErr != nil {
		return closeErr
	}
	fmt.Println("Database backup:", backupPath)
	hash := sha256.Sum256(content)
	batch := importBatch{ID: data.Batch, ManifestSHA256: hex.EncodeToString(hash[:]), SyntheticEngagement: true}
	var hot []struct {
		id    int64
		score int
	}
	err = gdb.Transaction(func(tx *gorm.DB) error {
		rng := mathrand.New(mathrand.NewSource(20261108))
		now := time.Now().UTC()
		curatorNames := []string{"峡谷放映员", "南风拾影", "林野的放映室", "天台看云", "半帧晚风", "阿橙影像簿", "一杯乌龙", "橘子电影机", "周末造梦", "纸月亮", "青柠宇宙", "山海来信"}
		curatorLogins := []string{"xiagu_movie", "nanfeng_frames", "linye_cinema", "tiantai_cloud", "wanfeng_half", "acheng_diary", "oolong_screen", "orange_cinema", "weekend_dream", "paper_moon", "lime_cosmos", "shanhai_letters"}
		prefix := []string{"晚风", "山野", "星河", "橘子", "南巷", "青禾", "月白", "云间", "林深", "落日", "初夏", "薄荷", "白昼", "松间", "冬青", "小岛", "远山", "浅湾", "晨雾", "花间", "半夏", "听雨", "拾光", "江边", "海盐", "北窗", "微蓝", "茶里", "轻舟", "木槿", "清泉", "时雨", "栗子", "竹里", "雪见", "风铃", "鹿鸣", "芥末", "日和", "青柠"}
		suffix := []string{"来信", "同学", "小记", "散步", "放映中", "慢半拍", "的咖啡", "不熬夜", "收藏夹", "看电影", "听故事", "在路上", "留一帧", "吃早饭", "等日落", "有回声", "写日记", "的周末", "与银河", "有点甜"}
		bios := []string{"留住喜欢的画面。", "喜欢有余味的故事。", "周末、电影和一杯热茶。", "收集脑洞，偶尔发呆。", "在影像里遇见不同的世界。", "最近在看 AI 短片，欢迎交流。"}
		users := make([]account.UserModel, 812)
		for i := range users {
			var secret [24]byte
			if _, err := rand.Read(secret[:]); err != nil {
				return err
			}
			name, login, bio := "", "", ""
			if i < len(curatorNames) {
				name, login, bio = curatorNames[i], curatorLogins[i], "收集喜欢的 AI 影像。作品原作者和来源见各视频详情。"
			} else {
				j := i - len(curatorNames)
				name = prefix[j%len(prefix)] + suffix[j/len(prefix)]
				login = fmt.Sprintf("film_%x", secret[:8])
				bio = bios[j%len(bios)]
			}
			created := now.Add(-time.Duration(14+rng.Intn(100)) * 24 * time.Hour)
			// Deliberately unloggable accounts: no shared/default passwords to expose.
			users[i] = account.UserModel{Account: login, Password: "!" + hex.EncodeToString(secret[:]), Nickname: name, Bio: bio, AvatarURL: fmt.Sprintf("/uploads/catalog/avatar/%02d.svg", i%24), Status: 1, Role: "user", CreatedAt: created, UpdatedAt: created}
		}
		if err := tx.CreateInBatches(&users, 200).Error; err != nil {
			return err
		}
		markers := make([]importAccount, len(users))
		for i, user := range users {
			markers[i] = importAccount{UserID: user.ID, BatchID: data.Batch}
		}
		if err := tx.CreateInBatches(&markers, 200).Error; err != nil {
			return err
		}
		batch.Accounts = len(users)
		var follows []relation.FollowModel
		following, followers := map[int64]int{}, map[int64]int{}
		for curator := range curatorNames {
			count := 38 + rng.Intn(210)
			for _, viewer := range rng.Perm(800)[:count] {
				from, to := users[viewer+12].ID, users[curator].ID
				at := now.Add(-time.Duration(rng.Intn(12*24)) * time.Hour)
				follows = append(follows, relation.FollowModel{UserID: from, TargetUserID: to, Status: 1, CreatedAt: at, UpdatedAt: at})
				following[from]++
				followers[to]++
			}
		}
		if err := tx.CreateInBatches(&follows, 300).Error; err != nil {
			return err
		}
		batch.Follows = len(follows)
		var relationStats []relation.RelationStatModel
		for _, user := range users {
			relationStats = append(relationStats, relation.RelationStatModel{UserID: user.ID, FollowingCount: following[user.ID], FollowerCount: followers[user.ID], CreatedAt: now, UpdatedAt: now})
		}
		if err := tx.CreateInBatches(&relationStats, 200).Error; err != nil {
			return err
		}
		comments := []string{"这个题材很适合做成一个系列。", "想看看下一集会怎么展开。", "这里的镜头衔接值得再看一次。", "留在收藏夹里，回头再慢慢看。", "更喜欢这种有完整故事的短片。", "这个开场很容易让人看进去。", "配乐和画面氛围搭得不错。", "好奇这一段分镜是怎么设计的。", "这一帧的光影挺有感觉。", "片尾还有回味的空间。", "原作者主页也值得去看看。", "故事之外，画面的细节也很有意思。", "短短几分钟，信息量还挺大。", "这个脑洞有意思，蹲后续。", "喜欢这种克制一点的叙事节奏。", "结尾这个停顿挺有想法。"}
		vectorizer := embedding.NewHashNgramVectorizer()
		for i, source := range data.Videos {
			creator := 1 + i%11
			if source.Category == "电竞短剧" {
				creator = 0
			}
			published := now.Add(-time.Duration(1+rng.Intn(10*24*60)) * time.Minute)
			key := data.Batch + ":" + source.BVID
			description := fmt.Sprintf("%s影像精选。原作者：B站 @%s。\n原作：%s", source.Category, source.SourceAuthor, source.SourceURL)
			model := video.VideoModel{AuthorID: users[creator].ID, Title: source.Title, Description: description, MediaURL: source.MediaURL, CoverURL: source.CoverURL, Status: 2, PublishedAt: &published, IdempotencyKey: &key, CreatedAt: published, UpdatedAt: published, AIStyleTag: &source.Category}
			if source.ModelName != "" {
				model.ModelName = &source.ModelName
			}
			if err := tx.Create(&model).Error; err != nil {
				return err
			}
			likes := 42 + rng.Intn(620)
			favorites := 6 + rng.Intn(60) + likes/7
			countComments := 3 + rng.Intn(12)
			if source.Category == "电竞短剧" {
				likes += 90
				favorites += 20
			}
			var actions []interaction.ActionModel
			for _, spec := range []struct {
				kind  string
				count int
			}{{"like", likes}, {"favorite", favorites}} {
				for _, index := range rng.Perm(800)[:spec.count] {
					at := published.Add(time.Duration(rng.Int63n(int64(now.Sub(published)))))
					k := key + ":" + spec.kind + fmt.Sprint(index)
					actions = append(actions, interaction.ActionModel{UserID: users[index+12].ID, VideoID: model.ID, ActionType: spec.kind, Status: 1, IdempotencyKey: &k, CreatedAt: at, UpdatedAt: at})
				}
			}
			if err := tx.CreateInBatches(&actions, 250).Error; err != nil {
				return err
			}
			var thread []interaction.CommentModel
			for j, index := range rng.Perm(800)[:countComments] {
				at := published.Add(time.Duration(rng.Int63n(int64(now.Sub(published)))))
				k := key + ":comment:" + fmt.Sprint(j)
				thread = append(thread, interaction.CommentModel{VideoID: model.ID, UserID: users[index+12].ID, Content: comments[(i*3+j)%len(comments)], Status: 1, IdempotencyKey: &k, CreatedAt: at, UpdatedAt: at})
			}
			if err := tx.Create(&thread).Error; err != nil {
				return err
			}
			stat := video.VideoStatModel{VideoID: model.ID, LikeCount: likes, FavoriteCount: favorites, CommentCount: countComments, CreatedAt: published, UpdatedAt: now}
			if err := tx.Create(&stat).Error; err != nil {
				return err
			}
			text := embedding.BuildVideoText(source.Title, description)
			vec, err := json.Marshal(vectorizer.Vectorize(text))
			if err != nil {
				return err
			}
			embed := embeddingmodel.VideoEmbeddingModel{VideoID: model.ID, Model: vectorizer.Model(), Dimension: vectorizer.Dimension(), EmbeddingJSON: string(vec), TextHash: embedding.TextHash(text), CreatedAt: published, UpdatedAt: now}
			if err := tx.Create(&embed).Error; err != nil {
				return err
			}
			provenance := importSource{VideoID: model.ID, BatchID: data.Batch, BVID: source.BVID, SourceURL: source.SourceURL, SourceAuthor: source.SourceAuthor, SourceStats: string(source.SourceStats)}
			if err := tx.Create(&provenance).Error; err != nil {
				return err
			}
			var inbox []feed.InboxModel
			for _, f := range follows {
				if f.TargetUserID == model.AuthorID {
					inbox = append(inbox, feed.InboxModel{UserID: f.UserID, VideoID: model.ID, AuthorID: model.AuthorID, PublishedAt: published, CreatedAt: now})
				}
			}
			if len(inbox) > 0 {
				if err := tx.CreateInBatches(&inbox, 200).Error; err != nil {
					return err
				}
			}
			hot = append(hot, struct {
				id    int64
				score int
			}{model.ID, likes*3 + favorites*4 + countComments*5})
			batch.Videos++
			batch.Likes += likes
			batch.Favorites += favorites
			batch.Comments += countComments
		}
		return tx.Create(&batch).Error
	})
	if err != nil {
		return fmt.Errorf("import rolled back: %w", err)
	}
	client, err := redis.NewRedisClient(context.Background(), cfg.Redis)
	if err != nil {
		fmt.Println("Import committed; Redis hot-feed warming failed:", err)
	} else {
		defer client.Close()
		c := cache.NewFeedCache(client)
		for _, item := range hot {
			if err := c.AddHotScore(context.Background(), item.id, item.score, time.Now()); err != nil {
				fmt.Println("Hot-feed warming:", err)
				break
			}
		}
	}
	fmt.Printf("Imported: %+v\n", batch)
	return nil
}
