package domainmessage

import "errors"

var (
	ErrInvalidUserID         = errors.New("user id must be positive")
	ErrInvalidMessageID      = errors.New("message id must be positive")
	ErrInvalidLimit          = errors.New("limit must be positive")
	ErrInvalidCursor         = errors.New("cursor is invalid")
	ErrInvalidMessageType    = errors.New("message type is invalid")
	ErrEmptyTitle            = errors.New("title is required")
	ErrTitleTooLong          = errors.New("title is too long")
	ErrEmptyContent          = errors.New("content is required")
	ErrContentTooLong        = errors.New("content is too long")
	ErrEventIDTooLong        = errors.New("event id is too long")
	ErrIdempotencyKeyTooLong = errors.New("idempotency key is too long")
	ErrMessageNotFound       = errors.New("message not found")
)
