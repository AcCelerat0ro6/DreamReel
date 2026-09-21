package interfacehttpvideo

import (
	domainvideo "DreamReel/internal/domain/video"
	"time"
)

type CreateVideoRequest struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	MediaURL    string `json:"media_url"`
	CoverURL    string `json:"cover_url"`
	// -- AIGC可选字段 --
	ModelName     *string `json:"model_name"`
	ModelParams   *string `json:"model_params"`
	AIStyleTag    *string `json:"ai_style_tag"`
	OriginVideoID *int64  `json:"origin_video_id"`
}

// videoResponse 是视频详情响应，包含视频主体字段和互动计数。
type videoCreateResponse struct {
	ID            int64      `json:"id"`
	AuthorID      int64      `json:"author_id"`
	Title         string     `json:"title"`
	Description   string     `json:"description"`
	MediaURL      string     `json:"media_url"`
	CoverURL      string     `json:"cover_url"`
	Status        int        `json:"status"`
	LikeCount     int        `json:"like_count"`
	CommentCount  int        `json:"comment_count"`
	FavoriteCount int        `json:"favorite_count"`
	PublishedAt   *time.Time `json:"published_at,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	// AIGC 元数据
	ModelName     *string `json:"model_name,omitempty"`
	ModelParams   *string `json:"model_params,omitempty"`
	AIStyleTag    *string `json:"ai_style_tag,omitempty"`
	OriginVideoID *int64  `json:"origin_video_id,omitempty"`
}

// videoResponseFromDomain 把领域视频转换成 HTTP JSON 响应。
func videoCreateResponseFromDomain(video *domainvideo.Video) videoCreateResponse {
	return videoCreateResponse{
		ID:            video.ID,
		AuthorID:      video.AuthorID,
		Title:         video.Title,
		Description:   video.Description,
		MediaURL:      video.MediaURL,
		CoverURL:      video.CoverURL,
		Status:        video.Status,
		LikeCount:     video.LikeCount,
		CommentCount:  video.CommentCount,
		FavoriteCount: video.FavoriteCount,
		PublishedAt:   video.PublishedAt,
		CreatedAt:     video.CreatedAt,
		UpdatedAt:     video.UpdatedAt,
		ModelName:     video.ModelName,
		ModelParams:   video.ModelParams,
		AIStyleTag:    video.AIStyleTag,
		OriginVideoID: video.OriginVideoID,
	}
}
