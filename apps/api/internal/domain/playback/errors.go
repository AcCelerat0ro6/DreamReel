package domainplayback

import "errors"

var (
	ErrInvalidUserID         = errors.New("user id must be non-negative")
	ErrInvalidVideoID        = errors.New("video id must be positive")
	ErrInvalidPlatform       = errors.New("platform is invalid")
	ErrInvalidNetworkType    = errors.New("network type is invalid")
	ErrInvalidLimit          = errors.New("limit must be positive")
	ErrInvalidFirstFrameMs   = errors.New("first frame ms must be non-negative")
	ErrInvalidStutterCount   = errors.New("stutter count must be non-negative")
	ErrInvalidWatchMs        = errors.New("watch ms must be non-negative")
	ErrIdempotencyKeyTooLong = errors.New("idempotency key is too long")
)
