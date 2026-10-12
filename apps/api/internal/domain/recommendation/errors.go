package domainrecommendation

import "errors"

var (
	ErrInvalidUserID    = errors.New("invalid user id")
	ErrInvalidVideoID   = errors.New("invalid video id")
	ErrInvalidLimit     = errors.New("invalid limit")
	ErrEmptyScene       = errors.New("scene is required")
	ErrSceneTooLong     = errors.New("scene is too long")
	ErrRequestIDTooLong = errors.New("request id is too long")
	ErrInvalidCursor    = errors.New("invalid cursor")
	ErrVideoNotFound    = errors.New("video not found")
)
