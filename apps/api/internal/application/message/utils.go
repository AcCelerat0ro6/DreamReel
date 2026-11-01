package applicationmessage

import (
	domainmessage "DreamReel/internal/domain/message"
	"encoding/base64"
	"encoding/json"
	"strings"
	"time"
)

const defaultMessageLimit = 20

func parseCursor(raw string) (*domainmessage.Cursor, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}

	content, err := base64.RawURLEncoding.DecodeString(raw)
	if err != nil {
		content, err = base64.StdEncoding.DecodeString(raw)
		if err != nil {
			return nil, domainmessage.ErrInvalidCursor
		}
	}

	var payload cursorPayload
	if err := json.Unmarshal(content, &payload); err != nil {
		return nil, domainmessage.ErrInvalidCursor
	}

	createdAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.CreatedAt))
	if err != nil || payload.MessageID <= 0 {
		return nil, domainmessage.ErrInvalidCursor
	}
	return &domainmessage.Cursor{CreatedAt: createdAt, MessageID: payload.MessageID}, nil
}

func normalizeLimit(limit int) int {
	if limit <= 0 {
		return defaultMessageLimit
	}
	if limit > domainmessage.MaxLimit {
		return domainmessage.MaxLimit
	}
	return limit
}

func encodeCursor(cursor *domainmessage.Cursor) string {
	if cursor == nil || cursor.MessageID <= 0 || cursor.CreatedAt.IsZero() {
		return ""
	}

	content, err := json.Marshal(cursorPayload{
		CreatedAt: cursor.CreatedAt.UTC().Format(time.RFC3339Nano),
		MessageID: cursor.MessageID,
	})
	if err != nil {
		return ""
	}
	return base64.RawURLEncoding.EncodeToString(content)
}
