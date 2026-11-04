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

// UpdateProfileRequest 用户资料更新请求
type UpdateProfileRequest struct {
	Nickname  *string `json:"nickname"`
	AvatarURL *string `json:"avatar_url"`
	Bio       *string `json:"bio"`
}

// userProfileResponse 用户信息响应
type userProfileResponse struct {
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

// publicUserProfileResponse 公开用户信息响应，隐藏账号、角色和状态等内部字段。
type publicUserProfileResponse struct {
	ID             int64  `json:"id"`
	Nickname       string `json:"nickname"`
	AvatarURL      string `json:"avatar_url"`
	Bio            string `json:"bio"`
	FollowingCount int    `json:"following_count"`
	FollowerCount  int    `json:"follower_count"`
	WorkCount      int    `json:"work_count"`
}

// profileResponse 将应用层 Profile 转成对外 JSON 结构。
func profileResponse(profile *applicationaccount.Profile) userProfileResponse {
	return userProfileResponse{
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

// publicProfileResponse 将应用层 Profile 转成公开 JSON 结构。
func publicProfileResponse(profile *applicationaccount.Profile) publicUserProfileResponse {
	return publicUserProfileResponse{
		ID:             profile.ID,
		Nickname:       profile.Nickname,
		AvatarURL:      profile.AvatarURL,
		Bio:            profile.Bio,
		FollowingCount: profile.FollowingCount,
		FollowerCount:  profile.FollowerCount,
		WorkCount:      profile.WorkCount,
	}
}
