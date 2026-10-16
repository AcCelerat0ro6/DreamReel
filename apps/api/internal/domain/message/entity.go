package domainmessage

const (
	TypeLike    = "LIKE"
	TypeComment = "COMMENT"
	TypeFollow  = "FOLLOW"
	TypeSystem  = "SYSTEM"

	MaxTitleLength          = 128
	MaxContentLength        = 1024
	MaxEventIDLength        = 64
	MaxIdempotencyKeyLength = 128
	MaxLimit                = 100
)
