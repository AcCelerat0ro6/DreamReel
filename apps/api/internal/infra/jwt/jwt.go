package jwt

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	jwtv5 "github.com/golang-jwt/jwt/v5"
)

const (
	defaultAccessTTL = 15 * time.Minute
	TokenTypeAccess  = "access"
)

var ErrEmptyJWTSecret = errors.New("jwt secret is required")
var ErrParseAccessTTL = errors.New("parse jwt access_ttl failed")
var ErrEmptyToken = errors.New("token is empty")
var ErrParseJWTToken = errors.New("parse jwt token failed")
var ErrTokenExpired = errors.New("jwt token expired")
var ErrTokenNotValidYet = errors.New("jwt token not valid yet")
var ErrTokenSignatureInvalid = errors.New("jwt token signature invalid")
var ErrTokenMalformed = errors.New("jwt token malformed")
var ErrInvalidTokenType = errors.New("token type invalid")
var ErrInvalidTokenUserID = errors.New("token user id invalid")
var ErrInvalidUserID = errors.New("user id must be positive")
var ErrGenerateTokenJTI = errors.New("generate token jti failed")
var ErrSignJWTToken = errors.New("sign jwt token failed")
var ErrInvalidTTL = errors.New("ttl must be positive")

// Manager 负责 JWT 签发和校验
type Manager struct {
	secret    []byte
	accessTTL time.Duration
}

// NewManager 解析secret及TTL,并初始化 JWT 管理器
func NewManager(secret, accessTTL string) (*Manager, error) {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return nil, ErrEmptyJWTSecret
	}

	accessDuration, err := parseTTL(accessTTL, defaultAccessTTL)
	if err != nil {
		return nil, ErrParseAccessTTL
	}

	return &Manager{
		secret:    []byte(secret),
		accessTTL: accessDuration,
	}, nil
}

// AccessTTL 返回access token 有效期
func (m *Manager) AccessTTL() time.Duration {
	return m.accessTTL
}

// SignAccessToken 签发访问 token
func (m *Manager) SignAccessToken(userID int64, role string) (string, error) {
	return m.signToken(userID, role, TokenTypeAccess, m.accessTTL)
}

// signToken 完成实际签发 token
func (m *Manager) signToken(userID int64, role, tokenType string, ttl time.Duration) (string, error) {
	if userID <= 0 {
		return "", ErrInvalidTokenUserID
	}

	role = strings.TrimSpace(role)
	if role == "" {
		role = "user"
	}
	now := time.Now()
	jti, err := randomID(16)
	if err != nil {
		return "", ErrGenerateTokenJTI
	}
	claims := tokenClaims{
		UserID:    userID,
		Role:      role,
		TokenType: tokenType,
		RegisteredClaims: jwtv5.RegisteredClaims{
			IssuedAt:  jwtv5.NewNumericDate(now),
			ExpiresAt: jwtv5.NewNumericDate(now.Add(ttl)),
			ID:        jti,
		},
	}

	token := jwtv5.NewWithClaims(jwtv5.SigningMethodHS256, claims)

	signedToken, err := token.SignedString(m.secret)
	if err != nil {
		return "", ErrSignJWTToken
	}

	return signedToken, nil
}

// ParseAndValidateToken 解析 token，并校验签名算法、过期时间和 token 类型。
func (m *Manager) ParseAndValidateToken(token, expectedType string) (*Claims, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrEmptyToken
	}

	parsedClaims := &tokenClaims{}
	_, err := jwtv5.ParseWithClaims(
		token,
		parsedClaims,
		func(token *jwtv5.Token) (any, error) {
			return m.secret, nil
		},
		// 限定签名算法可以避免算法降级类攻击。
		jwtv5.WithValidMethods([]string{jwtv5.SigningMethodHS256.Alg()}),
	)
	if err != nil {
		switch {
		case errors.Is(err, jwtv5.ErrTokenExpired):
			return nil, ErrTokenExpired
		case errors.Is(err, jwtv5.ErrTokenNotValidYet),
			errors.Is(err, jwtv5.ErrTokenUsedBeforeIssued):
			return nil, ErrTokenNotValidYet
		case errors.Is(err, jwtv5.ErrTokenSignatureInvalid),
			errors.Is(err, jwtv5.ErrTokenUnverifiable):
			return nil, ErrTokenSignatureInvalid
		case errors.Is(err, jwtv5.ErrTokenMalformed):
			return nil, ErrTokenMalformed
		default:
			return nil, ErrParseJWTToken
		}
	}

	if parsedClaims.TokenType != expectedType {
		return nil, ErrInvalidTokenType
	}
	if parsedClaims.UserID <= 0 {
		return nil, ErrInvalidTokenUserID
	}

	return &Claims{
		UserID:    parsedClaims.UserID,
		Role:      parsedClaims.Role,
		TokenType: parsedClaims.TokenType,
		JWTID:     parsedClaims.ID,
		IssuedAt:  claimTimeUnix(parsedClaims.IssuedAt),
		ExpiresAt: claimTimeUnix(parsedClaims.ExpiresAt),
	}, nil
}

// parseTTL 解析配置里的时间字符串，例如 15m、1h。
func parseTTL(raw string, fallback time.Duration) (time.Duration, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fallback, nil
	}
	ttl, err := time.ParseDuration(raw)
	if err != nil {
		return 0, err
	}
	if ttl <= 0 {
		return 0, ErrInvalidTTL
	}
	return ttl, nil
}

// randomID 生成十六进制随机串，用作 JWT ID。
func randomID(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

// claimTimeUnix 将 jwtv5.NumericDate 转成 Unix 秒，空值用 0 表示。
func claimTimeUnix(value *jwtv5.NumericDate) int64 {
	if value == nil {
		return 0
	}
	return value.Unix()
}
