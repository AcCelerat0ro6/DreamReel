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
