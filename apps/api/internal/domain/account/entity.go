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

// RestoreUserWithStats 从数据库记录恢复领域对象，并带上关系统计。
func RestoreUserWithStats(id int64, account, password, nickname, avatarURL, bio string, status int, role string, followingCount int, followerCount int, workCount int) *User {
	account = strings.TrimSpace(account)
	password = strings.TrimSpace(password)
	nickname = strings.TrimSpace(nickname)
	avatarURL = strings.TrimSpace(avatarURL)
	bio = strings.TrimSpace(bio)
	role = strings.TrimSpace(role)
	if status == 0 {
		status = StatusNormal
	}
	if role == "" {
		// 老数据或测试数据没有角色时，按普通用户处理。
		role = RoleUser
	}

	return &User{
		ID:             id,
		Account:        account,
		Password:       password,
		Nickname:       nickname,
		AvatarURL:      avatarURL,
		Bio:            bio,
		Status:         status,
		Role:           role,
		FollowingCount: clampCount(followingCount),
		FollowerCount:  clampCount(followerCount),
		WorkCount:      clampCount(workCount),
	}
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

func (u *User) Authenticate(password string) error {
	password = strings.TrimSpace(password)
	if password == "" {
		return ErrEmptyPassword
	}
	if err := bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(password)); err != nil {
		return ErrInvalidCredentials
	}
	return nil
}

func clampCount(value int) int {
	if value < 0 {
		return 0
	}
	return value
}
