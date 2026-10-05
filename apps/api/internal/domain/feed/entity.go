package domainfeed

import (
	"strings"
	"time"
)

// Scene 表示不同 Feed 场景，应用层通过场景选择对应策略。
type Scene string

// NormalizeScene 初始化 scene 参数格式，空值使用默认 Feed 场景。
func NormalizeScene(scene Scene) Scene {
	value := strings.TrimSpace(strings.ToLower(string(scene)))
	if value == "" ||
		(Scene(value) != SceneTimeline && Scene(value) != SceneRecommend && Scene(value) != SceneFollowing && Scene(value) != SceneHot) {
		return DefaultScene
	}
	return Scene(value)
}

const (
	DefaultFeedLimit = 10
	MaxFeedLimit     = 100

	SceneTimeline  Scene = "timeline"
	SceneRecommend Scene = "recommend"
	SceneFollowing Scene = "following"
	SceneHot       Scene = "hot"

	DefaultScene = SceneTimeline
)

// FeedItem 代表一个视频的卡片数据
type FeedItem struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
	LikeCount       int
	CommentCount    int
	FavoriteCount   int
	Liked           bool
	Favorited       bool
	HotScore        int
	PublishedAt     time.Time
	// AIGC 元数据
	ModelName *string
}

// FeedPage 代表一个视频的最小卡片数据，剩余的数据与缓存分开存储组装
type FeedPageItem struct {
	VideoID     int64
	AuthorID    int64
	PublishedAt time.Time
	HotScore    int
}

// TimelineCursor 保存时间线分页所需的排序字段。
type TimelineCursor struct {
	PublishedAt time.Time
	VideoID     int64
}

// HotCursor 保存热榜分页所需的排序字段。
type HotCursor struct {
	// 对应分支2. 无热度截止窗口，走基础数据累计热度
	HotScore    int
	PublishedAt time.Time
	VideoID     int64
	// 对应分支1. 有热度截止窗口，走Redis 最近一小时分钟桶
	WindowEnd time.Time
	Offset    int
}

// FeedCard 保存视频卡片中相对静态的展示字段
type FeedCard struct {
	VideoID         int64
	AuthorID        int64
	AuthorNickname  string
	AuthorAvatarURL string
	Title           string
	Description     string
	MediaURL        string
	CoverURL        string
	PublishedAt     time.Time
	ModelName       *string
}

// FeedStat 保存视频卡片中高频变更的字段
type FeedStat struct {
	VideoID       int64
	LikeCount     int
	CommentCount  int
	FavoriteCount int
}

// ViewerActionState 保存当前用户与当前这个视频的历史互动。
type ViewerActionState struct {
	VideoID   int64
	Liked     bool
	Favorited bool
}

// TurnCardAndStatIntoFeedItem 拼接Card和Stat并转换为供前端展示的FeedItem
func TurnCardAndStatIntoFeedItem(videoID int64, authorID int64, authorNickname string, authorAvatarURL string, title string, description string, mediaURL string, coverURL string, modelName *string, likeCount int, commentCount int, favoriteCount int, publishedAt time.Time) *FeedItem {
	return &FeedItem{
		VideoID:         videoID,
		AuthorID:        authorID,
		AuthorNickname:  strings.TrimSpace(authorNickname),
		AuthorAvatarURL: strings.TrimSpace(authorAvatarURL),
		Title:           strings.TrimSpace(title),
		Description:     strings.TrimSpace(description),
		MediaURL:        strings.TrimSpace(mediaURL),
		CoverURL:        strings.TrimSpace(coverURL),
		ModelName:       modelName,
		LikeCount:       likeCount,
		CommentCount:    commentCount,
		FavoriteCount:   favoriteCount,
		HotScore:        ScoreHotFeedItem(likeCount, commentCount, favoriteCount),
		PublishedAt:     publishedAt,
	}
}

// ScoreHotFeedItem 计算热榜排序分
func ScoreHotFeedItem(likeCount, commentCount, favoriteCount int) int {
	// 评论权重最高，收藏次之，点赞提供基础热度。
	return likeCount*3 + commentCount*5 + favoriteCount*4
}
