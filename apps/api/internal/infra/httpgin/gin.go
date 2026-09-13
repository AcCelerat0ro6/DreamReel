package httpgin

import (
	"context"
	"errors"
	"net/http"
	"strconv"
	"time"

	"DreamReel/internal/infra/config"
	"DreamReel/internal/infra/metrics"
	"DreamReel/internal/interfaces/http/middleware"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// shutdownTimeout 后端服务关停时等待在途请求处理完成的最长时间。
const shutdownTimeout = 10 * time.Second

// Init 创建 Gin 引擎，并挂载 Zap 日志、Recovery、Prometheus 指标中间件。
func Init(mode string) *gin.Engine {
	if mode != "" {
		gin.SetMode(mode)
	}
	g := gin.New()
	g.Use(middleware.GinLogger(), middleware.GinRecovery(true), metrics.HTTPMiddleware())
	return g
}

// Run 启动 HTTP 服务，并且在收到 ctx 取消信号后执行关停。
// 关停顺序: 停止接受新连接 -> 在有限时间内等待在途请求完成 -> 返回。
func Run(ctx context.Context, cfg *config.Config, g *gin.Engine) error {
	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           g,
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case err := <-errCh:
		return err
	case <-ctx.Done():
		zap.L().Info("收到关停信号，正在关闭后端服务...")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		if err := srv.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return <-errCh
	}
}
