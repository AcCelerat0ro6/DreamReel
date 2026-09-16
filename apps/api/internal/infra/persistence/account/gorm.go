package infraaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"context"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
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

// New 创建账号仓储实现，db 由路由装配阶段注入。
func New(db *gorm.DB) *Repository {
	return &Repository{db: db}
}

// Save 创建账户, 进行实际写入 account 表操作
func (r *Repository) Save(ctx context.Context, user *domainaccount.User) error {
	model := UserModel{
		Account:   user.Account,
		AvatarURL: user.AvatarURL,
		Bio:       user.Bio,
		Nickname:  user.Nickname,
		Password:  user.Password,
		Role:      user.Role,
		Status:    user.Status,
	}

	if err := r.db.WithContext(ctx).Create(&model).Error; err != nil {
		if isDuplicateKeyError(err) {
			return domainaccount.ErrAccountAlreadyExists
		}
		return err
	}
	// 注意: 回填用户ID 至 domain层模型
	user.ID = model.ID
	return nil
}
