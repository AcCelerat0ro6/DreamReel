package interfacehttpfeed

import (
	applicationfeed "DreamReel/internal/application/feed"
	"time"
)

// feedQueryRequest 是复杂 Feed 查询入口的请求体。
type feedQueryRequest struct {
	Scene         string            `json:"scene"`
	Cursor        string            `json:"cursor"`
	Limit         *int              `json:"limit"`
	ClientContext map[string]string `json:"context"`
}

// feedItemsResponse 是 HTTP层 Feed 游标分页响应。
type feedItemsResponse struct {
	Scene      string             `json:"scene"`
	Items      []feedItemResponse `json:"items"`
	NextCursor string             `json:"next_cursor"`
	HasMore    bool               `json:"has_more"`
}

// feedItemResponse 是 Feed 中单条视频卡片的响应结构。
type feedItemResponse struct {
	VideoID         int64     `json:"video_id"`
	AuthorID        int64     `json:"author_id"`
	AuthorNickname  string    `json:"author_nickname"`
	AuthorAvatarURL string    `json:"author_avatar_url"`
	Title           string    `json:"title"`
	Description     string    `json:"description"`
	MediaURL        string    `json:"media_url"`
	CoverURL        string    `json:"cover_url"`
	LikeCount       int       `json:"like_count"`
	CommentCount    int       `json:"comment_count"`
	FavoriteCount   int       `json:"favorite_count"`
	Liked           bool      `json:"liked"`
	Favorited       bool      `json:"favorited"`
	PublishedAt     time.Time `json:"published_at"`
	ModelName       *string   `json:"model_name,omitempty"`
	ModelParams     *string   `json:"model_params,omitempty"`
	AIStyleTag      *string   `json:"ai_style_tag,omitempty"`
	OriginVideoID   *int64    `json:"origin_video_id,omitempty"`
}

// TurnApplicationResultIntoResponse 把应用层 Feed 结果转换为 HTTP 响应结构。
func TurnApplicationResultIntoResponse(result *applicationfeed.FeedResult) *feedItemsResponse {
	items := make([]feedItemResponse, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, feedItemResponse{
			VideoID:         item.VideoID,
			AuthorID:        item.AuthorID,
			AuthorNickname:  item.AuthorNickname,
			AuthorAvatarURL: item.AuthorAvatarURL,
			Title:           item.Title,
			Description:     item.Description,
			MediaURL:        item.MediaURL,
			CoverURL:        item.CoverURL,
			LikeCount:       item.LikeCount,
			CommentCount:    item.CommentCount,
			FavoriteCount:   item.FavoriteCount,
			Liked:           item.Liked,
			Favorited:       item.Favorited,
			PublishedAt:     item.PublishedAt,
			ModelName:       item.ModelName,
			ModelParams:     item.ModelParams,
			AIStyleTag:      item.AIStyleTag,
			OriginVideoID:   item.OriginVideoID,
		})
	}
	return &feedItemsResponse{
		Scene:      string(result.Scene),
		Items:      items,
		NextCursor: result.NextCursor,
		HasMore:    result.HasMore,
	}
}
