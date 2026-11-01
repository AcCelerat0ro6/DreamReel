package interfacehttpinteraction

import (
	domaininteraction "DreamReel/internal/domain/interaction"
	"time"
)

// actionResponse 是点赞/收藏状态变更后的响应。
type actionResponse struct {
	VideoID       int64  `json:"video_id"`
	ActionType    string `json:"action_type"`
	Active        bool   `json:"active"`
	LikeCount     int    `json:"like_count"`
	FavoriteCount int    `json:"favorite_count"`
}

// createCommentRequest 接收评论内容
type createCommentRequest struct {
	Content string `json:"content"`
}

// createCommentResponse 创建评论的实际返回响应
type commentResponse struct {
	ID            int64     `json:"id"`
	VideoID       int64     `json:"video_id"`
	UserID        int64     `json:"user_id"`
	UserNickname  string    `json:"user_nickname"`
	UserAvatarURL string    `json:"user_avatar_url"`
	Content       string    `json:"content"`
	CreatedAt     time.Time `json:"created_at"`
	CommentCount  int       `json:"comment_count,omitempty"`
}

type CreateCommentResult struct {
	Comment      *domaininteraction.Comment
	CommentCount int
}

// commentListResponse 是评论游标分页响应。
type commentListResponse struct {
	Items      []commentResponse `json:"items"`
	NextCursor string            `json:"next_cursor"`
	HasMore    bool              `json:"has_more"`
}

// deleteCommentResponse 是删除评论后的状态响应。
type deleteCommentResponse struct {
	CommentID    int64 `json:"comment_id"`
	Status       int   `json:"status"`
	CommentCount int   `json:"comment_count"`
}
