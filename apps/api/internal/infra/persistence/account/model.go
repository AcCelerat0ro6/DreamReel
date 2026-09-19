package infraaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"time"
)

// UserModel 映射 account 表，保存用户账户信息
type UserModel struct {
	ID        int64     `gorm:"column:id;primaryKey;autoIncrement"`
	Account   string    `gorm:"column:account;size:64;not null;uniqueIndex"`
	Password  string    `gorm:"column:password;size:255;not null"`
	Nickname  string    `gorm:"column:nickname;size:128;not null"`
	AvatarURL string    `gorm:"column:avatar_url;size:512"`
	Bio       string    `gorm:"column:bio;size:255"`
	Status    int       `gorm:"column:status;not null;default:1"`
	Role      string    `gorm:"column:role;size:32;not null"`
	CreatedAt time.Time `gorm:"column:created_at;autoCreateTime"`
	UpdatedAt time.Time `gorm:"column:updated_at;autoUpdateTime"`
}

func (UserModel) TableName() string {
	return "account"
}

type userWithStatModel struct {
	ID             int64
	Account        string
	Password       string
	Nickname       string
	AvatarURL      string
	Bio            string
	Status         int
	Role           string
	FollowingCount int
	FollowerCount  int
	WorkCount      int
}

// restoreUser 把数据库模型转换回领域对象。
func restoreUser(user userWithStatModel) *domainaccount.User {
	return domainaccount.RestoreUserWithStats(
		user.ID,
		user.Account,
		user.Password,
		user.Nickname,
		user.AvatarURL,
		user.Bio,
		user.Status,
		user.Role,
		user.FollowingCount,
		user.FollowerCount,
		user.WorkCount,
	)
}
