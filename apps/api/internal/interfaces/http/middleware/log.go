package middleware

import (
	"fmt"
	"net"
	"net/http"
	"net/http/httputil"
	"os"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

//	两个日志中间件，替换Gin默认的Recovery和Logger
//
// GinLogger 接收 Gin 框架默认的日志，替换原生的 gin.Logger()
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 执行下一个中间件及路由处理函数
		c.Next()

		cost := time.Since(start)

		// 记录请求相关信息
		zap.L().Info(path,
			zap.Int("status", c.Writer.Status()),                                 // HTTP 状态码
			zap.String("method", c.Request.Method),                               // 请求方法
			zap.String("path", path),                                             // 请求路径
			zap.String("query", query),                                           // URL 参数
			zap.String("ip", c.ClientIP()),                                       // 客户端 IP
			zap.String("user-agent", c.Request.UserAgent()),                      // UserAgent
			zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()), // 内部错误信息
			zap.Duration("cost", cost),                                           // 耗时
		)
	}
}

// GinRecovery 捕获项目中可能出现的 panic，替换原生的 gin.Recovery()
// stack 参数决定是否在日志中记录完整的堆栈追踪信息
func GinRecovery(stack bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				// 获取产生 panic 的请求信息
				httpRequest, _ := httputil.DumpRequest(c.Request, false)

				// 检查是否为底层的断开连接错误（Broken pipe）
				// 如果客户端断开了连接，这个时候我们不应该继续向其返回状态码
				var brokenPipe bool
				if ne, ok := err.(*net.OpError); ok {
					if se, ok := ne.Err.(*os.SyscallError); ok {
						if strings.Contains(strings.ToLower(se.Error()), "broken pipe") || strings.Contains(strings.ToLower(se.Error()), "connection reset by peer") {
							brokenPipe = true
						}
					}
				}

				if brokenPipe {
					zap.L().Error(c.Request.URL.Path,
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
					)
					if e, ok := err.(error); ok {
						c.Error(e)
					} else {
						c.Error(fmt.Errorf("%v", err))
					}

					c.Abort()
					return
				}

				// 根据是否需要记录堆栈信息，决定日志打印的详细程度
				if stack {
					zap.L().Error("[Recovery from panic]",
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
						zap.String("stack", string(debug.Stack())), // 记录堆栈信息
					)
				} else {
					zap.L().Error("[Recovery from panic]",
						zap.Any("error", err),
						zap.String("request", string(httpRequest)),
					)
				}

				// 向客户端返回 500 内部服务器错误
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		// 执行后续业务逻辑
		c.Next()
	}
}
