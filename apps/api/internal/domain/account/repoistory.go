package domainaccount

import (
	"context"
)

// Repository接口仅定义账号领域需要的持久化能力
type Repository interface {
	// 添加新账户
	Save(ctx context.Context, user *User) error
	// FindUserByAccount 通过账户名查找用户
	FindUserByAccount(ctx context.Context, account string) (*User, error)
}
