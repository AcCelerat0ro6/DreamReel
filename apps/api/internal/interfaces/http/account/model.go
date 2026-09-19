package interfaceshttpaccount

import applicationaccount "DreamReel/internal/application/account"

// 注册请求结构体
type RegisterRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
	Nickname string `json:"nickname"`
}

// 注册后用户信息响应结构体
type RegisterResponse struct {
	ID             int64  `json:"id"`
	Account        string `json:"account"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	Status         int    `json:"status"`
	Role           string `json:"role"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}

// TurnProfileIntoResponse 将应用层 Profile 转成对外 JSON 结构。
func TurnProfileIntoResponse(profile *applicationaccount.Profile) RegisterResponse {
	return RegisterResponse{
		ID:             profile.ID,
		Account:        profile.Account,
		Nickname:       profile.Nickname,
		AvatarURL:      profile.AvatarURL,
		Bio:            profile.Bio,
		Status:         profile.Status,
		Role:           profile.Role,
		FollowingCount: profile.FollowingCount,
		FollowerCount:  profile.FollowerCount,
		WorkCount:      profile.WorkCount,
	}
}

// 登录请求结构体
type LoginByPasswordRequest struct {
	Account  string `json:"account"`
	Password string `json:"password"`
}

// 登录响应结构体
// 账号登录响应
type loginResponse struct {
	AccessToken      string `json:"access_token"`
	TokenType        string `json:"token_type"`
	ExpiresInSeconds int64  `json:"expires_in_seconds"`
}
