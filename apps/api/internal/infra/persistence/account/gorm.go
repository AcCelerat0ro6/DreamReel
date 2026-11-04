package infraaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"context"
	"errors"

	"gorm.io/gorm"
)

type Repository struct {
	db *gorm.DB
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

// FindByAccount 根据账号查找用户，登录流程会调用它。
func (r *Repository) FindByAccount(ctx context.Context, account string) (*domainaccount.User, error) {
	var user userWithStatModel
	err := r.db.WithContext(ctx).
		Table("account AS a").
		Select(userWithStatSelect()).
		Joins("LEFT JOIN user_relation_stat AS rs ON rs.user_id = a.id").
		Joins("LEFT JOIN (SELECT user_id, COUNT(*) AS following_count FROM user_follow WHERE status = 1 GROUP BY user_id) AS active_following ON active_following.user_id = a.id").
		Joins("LEFT JOIN (SELECT target_user_id, COUNT(*) AS follower_count FROM user_follow WHERE status = 1 GROUP BY target_user_id) AS active_followers ON active_followers.target_user_id = a.id").
		Joins("LEFT JOIN (SELECT author_id, COUNT(*) AS work_count FROM video WHERE status = 2 GROUP BY author_id) AS published_works ON published_works.author_id = a.id").
		Where("a.account = ?", account).
		Take(&user).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainaccount.ErrUserNotFound
		}
		return nil, err
	}
	return restoreUser(user), nil
}

// FindByID 根据用户 ID 查找用户，鉴权后的个人资料接口与公开主页接口会调用它。
func (r *Repository) FindByID(ctx context.Context, id int64) (*domainaccount.User, error) {
	var user userWithStatModel
	err := r.db.WithContext(ctx).
		Table("account AS a").
		Select(userWithStatSelect()).
		Joins("LEFT JOIN user_relation_stat AS rs ON rs.user_id = a.id").
		Joins("LEFT JOIN (SELECT user_id, COUNT(*) AS following_count FROM user_follow WHERE status = 1 GROUP BY user_id) AS active_following ON active_following.user_id = a.id").
		Joins("LEFT JOIN (SELECT target_user_id, COUNT(*) AS follower_count FROM user_follow WHERE status = 1 GROUP BY target_user_id) AS active_followers ON active_followers.target_user_id = a.id").
		Joins("LEFT JOIN (SELECT author_id, COUNT(*) AS work_count FROM video WHERE status = 2 GROUP BY author_id) AS published_works ON published_works.author_id = a.id").
		Where("a.id = ?", id).
		Take(&user).
		Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, domainaccount.ErrUserNotFound
		}
		return nil, err
	}
	return restoreUser(user), nil
}

// UpdateProfile 只更新资料字段，账号、密码、角色等字段保持原值。
func (r *Repository) UpdateProfile(ctx context.Context, user *domainaccount.User) error {
	result := r.db.WithContext(ctx).
		Model(&UserModel{}).
		Where("id = ?", user.ID).
		Updates(map[string]any{
			"nickname":   user.Nickname,
			"avatar_url": user.AvatarURL,
			"bio":        user.Bio,
		})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return domainaccount.ErrUserNotFound
	}
	return nil
}

func userWithStatSelect() string {
	return "a.id, a.account, a.password, a.nickname, a.avatar_url, a.bio, a.status, a.role, COALESCE(active_following.following_count, rs.following_count, 0) AS following_count, COALESCE(active_followers.follower_count, rs.follower_count, 0) AS follower_count, COALESCE(published_works.work_count, 0) AS work_count"
}
