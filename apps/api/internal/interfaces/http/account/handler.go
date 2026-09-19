package interfaceshttpaccount

import (
	applicationaccount "DreamReel/internal/application/account"
	domainaccount "DreamReel/internal/domain/account"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

type Handler struct {
	service *applicationaccount.Service
}

func New(service *applicationaccount.Service) *Handler {
	return &Handler{service: service}
}

// Register 处理用户注册请求，成功后返回新用户资料。
func (h *Handler) Register(c *gin.Context) {
	var req RegisterRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid register request param"})
		return
	}

	profile, err := h.service.Register(c.Request.Context(), req.Account, req.Password, req.Nickname)
	if err != nil {
		if isBadRequestError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		} else if errors.Is(err, domainaccount.ErrAccountAlreadyExists) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		} else {
			// 对内记录详细的错误，对外仅返回通用的服务器内部错误信息
			zap.L().Error("Internal Error", zap.Error(err))
			c.JSON(http.StatusInternalServerError, gin.H{"error": "Internal Server Error"})
		}
		return
	}

	c.JSON(http.StatusCreated, TurnProfileIntoResponse(profile))

}

// isBadRequestError 判断哪些领域错误属于客户端请求参数问题。
func isBadRequestError(err error) bool {
	return errors.Is(err, domainaccount.ErrEmptyAccount) ||
		errors.Is(err, domainaccount.ErrEmptyPassword) ||
		errors.Is(err, domainaccount.ErrEmptyNickname) ||
		errors.Is(err, domainaccount.ErrInvalidUserID) ||
		errors.Is(err, domainaccount.ErrEmptyProfileUpdate)
}

// Login 登录处理
func (h *Handler) Login(c *gin.Context) {
	var req LoginByPasswordRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid login request param"})
		return
	}

	token, err := h.service.Login(c.Request.Context(), req.Account, req.Password)
	if err != nil {
		if isBadRequestError(err) {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		if errors.Is(err, domainaccount.ErrInvalidCredentials) {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid credentials"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		zap.L().Info("Internal Error While Login:", zap.Error(err))
		return
	}

	c.JSON(http.StatusOK, loginResponse{
		AccessToken:      token.AccessToken,
		TokenType:        token.TokenType,
		ExpiresInSeconds: token.ExpiresInSeconds,
	})
}

// Logout 登出处理
func (h *Handler) Logout(c *gin.Context) {
	c.Status(http.StatusNoContent)
}
