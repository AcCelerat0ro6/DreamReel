package domainaccount

import (
	"strings"

	"golang.org/x/crypto/bcrypt"
)

const (
	RoleUser     = "user"
	RoleAdmin    = "admin"
	StatusNormal = 1
)

// User 是账号聚合根，保存登录凭证、展示资料和权限角色。
type User struct {
	ID        int64 //
	Account   string
	Password  string
	Nickname  string
	AvatarURL string
	Bio       string
	Status    int
	Role      string
	// FollowingCount 和 FollowerCount 来自关系模块统计表，用于个人页展示。
	FollowingCount int
	FollowerCount  int
	WorkCount      int
}

func New(account, password, nickname string) (*User, error) {
	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	nickname = strings.TrimSpace(nickname)

	if account == "" {
		return nil, ErrEmptyAccount
	}
	if password == "" {
		return nil, ErrEmptyPassword
	}
	if nickname == "" {
		return nil, ErrEmptyNickname
	}

	hashedPassword, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return nil, ErrHashPasswordFailed
	}

	return &User{
		Account:  account,
		Password: string(hashedPassword),
		Nickname: nickname,
		Status:   StatusNormal,
		Role:     RoleUser,
	}, nil
}
