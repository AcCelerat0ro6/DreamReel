package jwt

import (
	jwtv5 "github.com/golang-jwt/jwt/v5"
)

// Claims 存储 JWT Token信息 供业务读取
type Claims struct {
	UserID    int64  `json:"uid"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	JWTID     string `json:"jti"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

// tokenClaims 存储 JWT Token信息，是真正写入JWT的声明
type tokenClaims struct {
	UserID    int64  `json:"uid"`
	Role      string `json:"role"`
	TokenType string `json:"token_type"`
	jwtv5.RegisteredClaims
}
