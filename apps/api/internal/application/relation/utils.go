package applicationrelation

import (
	domainrelation "DreamReel/internal/domain/relation"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const defaultListLimit = 20

type listCursorPayload struct {
	FollowedAt string `json:"followed_at"`
	UserID     int64  `json:"user_id"`
}

func parseListCursor(raw string) (*domainrelation.ListCursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainrelation.ErrInvalidCursor
		}
	}

	var payload listCursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainrelation.ErrInvalidCursor
	}

	followedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.FollowedAt))
	if err != nil || payload.UserID <= 0 {
		return nil, domainrelation.ErrInvalidCursor
	}

	return &domainrelation.ListCursor{
		FollowedAt: followedAt,
		UserID:     payload.UserID,
	}, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultListLimit
	}
	if limit > domainrelation.MaxLimit {
		return domainrelation.MaxLimit
	}
	return limit
}

func listResult(items []*domainrelation.UserItem, limit int) *ListResult {
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}

	nextCursor := ""
	if len(items) > 0 {
		last := items[len(items)-1]
		nextCursor = encodeListCursor(&domainrelation.ListCursor{
			FollowedAt: last.FollowedAt,
			UserID:     last.UserID,
		})
	}

	return &ListResult{
		Items:      items,
		NextCursor: nextCursor,
		HasMore:    hasMore,
	}
}

func encodeListCursor(cursor *domainrelation.ListCursor) string {
	if cursor == nil || cursor.UserID <= 0 || cursor.FollowedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(listCursorPayload{
		FollowedAt: cursor.FollowedAt.UTC().Format(time.RFC3339Nano),
		UserID:     cursor.UserID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
