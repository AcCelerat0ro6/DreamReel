package domainvideo

import (
	"strings"
	"time"
)

const (
	StatusDraft     = 1
	StatusPublished = 2
	StatusOffline   = 3
	StatusDeleted   = 4

	MaxTitleLength          = 128
	MaxDescriptionLength    = 512
	MaxIdempotencyKeyLength = 128

	MaxModelNameLength   = 64
	MaxModelParamsLength = 1024
	MaxAIStyleTagLength  = 32
)

type Video struct {
	ID             int64
	AuthorID       int64
	Title          string
	Description    string
	MediaURL       string
	CoverURL       string
	Status         int
	LikeCount      int
	CommentCount   int
	FavoriteCount  int
	PublishedAt    *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
	IdempotencyKey *string
	// AIGC 元数据
	ModelName     *string
	ModelParams   *string
	AIStyleTag    *string
	OriginVideoID *int64
}

// DeleteBy 执行作者权限校验并把视频置为删除状态。
func (v *Video) DeleteBy(authorID int64) error {
	if authorID <= 0 {
		return ErrInvalidAuthorID
	}
	if v.AuthorID != authorID {
		return ErrVideoPermissionDenied
	}
	// 注意：删除采用软删除，保留原始记录用于审计、统计或后续恢复。
	if v.Status == StatusDeleted {
		return nil
	}
	v.Status = StatusDeleted
	return nil
}

// NewPublished 新建发布消息模型
func NewPublished(authorID int64, title, description, mediaURL, coverURL string, modelName, modelParams, aiStyleTag *string, originVideoID *int64, idempotencyKey string) (*Video, error) {
	if authorID <= 0 {
		return nil, ErrInvalidAuthorID
	}
	title = strings.TrimSpace(title)
	description = strings.TrimSpace(description)
	mediaURL = strings.TrimSpace(mediaURL)
	coverURL = strings.TrimSpace(coverURL)
	idempotencyKey = strings.TrimSpace(idempotencyKey)

	if title == "" {
		return nil, ErrEmptyTitle
	}
	if len(title) > MaxTitleLength {
		return nil, ErrTitleTooLong
	}
	if len(description) > MaxDescriptionLength {
		return nil, ErrDescriptionTooLong
	}
	if mediaURL == "" {
		return nil, ErrEmptyMediaURL
	}
	if coverURL == "" {
		return nil, ErrEmptyCoverURL
	}
	if len(idempotencyKey) > MaxIdempotencyKeyLength {
		return nil, ErrIdempotencyKeyTooLong
	}
	var idempotencyKeyPtr *string = &idempotencyKey
	if len(idempotencyKey) == 0 {
		idempotencyKeyPtr = nil
	}

	// AIGC 元数据归一化与长度校验（所有字段可选）
	normalizedModelName, err := normalizeOptionalString(modelName, MaxModelNameLength, ErrModelNameTooLong)
	if err != nil {
		return nil, err
	}
	normalizedModelParams, err := normalizeOptionalString(modelParams, MaxModelParamsLength, ErrModelParamsTooLong)
	if err != nil {
		return nil, err
	}
	normalizedAIStyleTag, err := normalizeOptionalString(aiStyleTag, MaxAIStyleTagLength, ErrAIStyleTagTooLong)
	if err != nil {
		return nil, err
	}

	now := time.Now()
	video := &Video{
		AuthorID:       authorID,
		Title:          title,
		Description:    description,
		MediaURL:       mediaURL,
		CoverURL:       coverURL,
		Status:         StatusPublished,
		PublishedAt:    &now,
		IdempotencyKey: idempotencyKeyPtr,
		ModelName:      normalizedModelName,
		ModelParams:    normalizedModelParams,
		AIStyleTag:     normalizedAIStyleTag,
		OriginVideoID:  originVideoID,
	}
	return video, nil
}

// normalizeOptionalString 去除可选字符串首尾空白并校验最大长度；空白字符串归一化为 nil。
func normalizeOptionalString(raw *string, maxLen int, tooLongErr error) (*string, error) {
	if raw == nil {
		return nil, nil
	}
	trimmed := strings.TrimSpace(*raw)
	if trimmed == "" {
		return nil, nil
	}
	if len(trimmed) > maxLen {
		return nil, tooLongErr
	}
	return &trimmed, nil
}
