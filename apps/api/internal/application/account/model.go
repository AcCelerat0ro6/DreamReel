package applicationaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"time"
)

func TurnUserIntoProfile(user *domainaccount.User) *Profile {
	return &Profile{
		ID:             user.ID,
		Account:        user.Account,
		AvatarURL:      user.AvatarURL,
		Bio:            user.Bio,
		Nickname:       user.Nickname,
		Role:           user.Role,
		Status:         user.Status,
		WorkCount:      user.WorkCount,
		FollowerCount:  user.FollowerCount,
		FollowingCount: user.FollowingCount,
	}
}

// Profile 是实际返回给前端的模型，屏蔽密码敏感字段
type Profile struct {
	ID             int64
	Account        string
	Nickname       string
	AvatarURL      string
	Bio            string
	Status         int
	Role           string
	FollowingCount int
	FollowerCount  int
	WorkCount      int
}

// LoginResult 包含了登录成功后返回给 HTTP 层的 token 数据。
type LoginResult struct {
	AccessToken      string
	TokenType        string
	ExpiresInSeconds int64
}

type TokenSigner interface {
	// 签发Access Token
	SignAccessToken(userID int64, role string) (string, error)
	// 解析Access Token 过期实践
	AccessTTL() time.Duration
}
