package applicationaccount

import (
	domainaccount "DreamReel/internal/domain/account"
	"context"
	"errors"
	"strings"
)

var ErrLoadAccountFailed = errors.New("failed to load account")
var ErrSaveAccountFailed = errors.New("failed to save account")
var ErrUpdateAccountFailed = errors.New("failed to update account")
var ErrSignAccessTokenFailed = errors.New("failed to sign access token")

type Service struct {
	repo   domainaccount.Repository
	signer TokenSigner
}

func New(repo domainaccount.Repository, signer TokenSigner) *Service {
	return &Service{repo: repo, signer: signer}
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

// Login 用户登录
func (s *Service) Login(ctx context.Context, account, password string) (*LoginResult, error) {
	account = strings.TrimSpace(account)
	if account == "" {
		return nil, domainaccount.ErrEmptyAccount
	}

	user, err := s.repo.FindUserByAccount(ctx, account)
	if err != nil {
		// 判断是否为账号不存在的错误
		if errors.Is(err, domainaccount.ErrUserNotFound) {
			// 注意 这里返回的是 ErrInvalidCredentials 而不是 ErrUserNotFound
			return nil, domainaccount.ErrInvalidCredentials
		}
		return nil, ErrLoadAccountFailed
	}
	if err := user.Authenticate(password); err != nil {
		return nil, err
	}

	if user.Status != domainaccount.StatusNormal {
		return nil, domainaccount.ErrAccountDisabled
	}

	accessToken, err := s.signer.SignAccessToken(user.ID, user.Role)
	if err != nil {
		return nil, err
	}

	return &LoginResult{
		AccessToken:      accessToken,
		TokenType:        "Bearer",
		ExpiresInSeconds: int64(s.signer.AccessTTL().Seconds()),
	}, nil
}
