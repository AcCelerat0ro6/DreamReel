package applicationmessage

import domainmessage "DreamReel/internal/domain/message"

type ListResult struct {
	Items      []*domainmessage.Message
	NextCursor string
	HasMore    bool
}

type cursorPayload struct {
	CreatedAt string `json:"created_at"`
	MessageID int64  `json:"message_id"`
}

type UnreadStat struct {
	UnreadCount int
}

// CreateResult 定义创建Message 的返回结果
type CreateResult struct {
	Message *domainmessage.Message
	Created bool
}
