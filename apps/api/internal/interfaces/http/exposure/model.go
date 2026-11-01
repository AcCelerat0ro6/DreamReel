package interfacehttpexposure

import "time"

// CreateViewEventRequest 创建视频曝光和观看行为上报请求
type createViewEventRequest struct {
	VideoID   int64  `json:"video_id"`
	Scene     string `json:"scene"`
	RequestID string `json:"request_id"`
	EventType string `json:"event_type"`
	WatchMs   int    `json:"watch_ms"`
	Completed bool   `json:"completed"`
}

// createViewEventResponse 是上报请求对应的的完整响应。
type createViewEventResponse struct {
	Event    viewEventResponse `json:"event"`
	Exposure *exposureResponse `json:"exposure,omitempty"`
}

// viewEventResponse 是观看行为响应。
type viewEventResponse struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	VideoID   int64     `json:"video_id"`
	Scene     string    `json:"scene"`
	RequestID string    `json:"request_id,omitempty"`
	EventType string    `json:"event_type"`
	WatchMs   int       `json:"watch_ms"`
	Completed bool      `json:"completed"`
	CreatedAt time.Time `json:"created_at"`
}

// exposureResponse 是曝光聚合响应
type exposureResponse struct {
	UserID         int64     `json:"user_id"`
	VideoID        int64     `json:"video_id"`
	FirstExposedAt time.Time `json:"first_exposed_at"`
	LastExposedAt  time.Time `json:"last_exposed_at"`
	ExposureCount  int       `json:"exposure_count"`
	LastScene      string    `json:"last_scene"`
}
