package interfacehttpinteraction

// actionResponse 是点赞/收藏状态变更后的响应。
type actionResponse struct {
	VideoID       int64  `json:"video_id"`
	ActionType    string `json:"action_type"`
	Active        bool   `json:"active"`
	LikeCount     int    `json:"like_count"`
	FavoriteCount int    `json:"favorite_count"`
}
