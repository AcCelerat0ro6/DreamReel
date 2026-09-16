package applicationaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"context"
	"errors"
)

var ErrLoadAccountFailed = errors.New("failed to load account")
var ErrSaveAccountFailed = errors.New("failed to save account")
var ErrUpdateAccountFailed = errors.New("failed to update account")
var ErrSignAccessTokenFailed = errors.New("failed to sign access token")

type Service struct {
	repo domainaccount.Repository
}

func New(repo domainaccount.Repository) *Service {
	return &Service{repo: repo}
}

// Register 创建新用户
func (s *Service) Register(ctx context.Context, account, password, nickname string) (*Profile, error) {
	user, err := domainaccount.New(account, password, nickname)

	if err != nil {
		return nil, err
	}

	err = s.repo.Save(ctx, user)
	if err != nil {
		if errors.Is(err, domainaccount.ErrAccountAlreadyExists) {
			return nil, domainaccount.ErrAccountAlreadyExists
		}
		return nil, ErrSaveAccountFailed
	}

	return TurnUserIntoProfile(user), nil
}
