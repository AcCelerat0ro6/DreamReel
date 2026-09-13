package log

import (
	"os"

	"DreamReel/internal/infra/config"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

// Init 初始化全局Logger
func Init(loggercfg *config.LoggerConfig) (err error) {
	// 1. 获取日志写入器 (整合 lumberjack 日志切割)
	writeSyncer := getLogWriter(
		loggercfg.Filename,
		loggercfg.MaxSize,
		loggercfg.MaxBackups,
		loggercfg.MaxAge,
	)

	// 2. 获取日志编码器 (定义日志的输出格式)
	encoder := getEncoder()

	// 3. 解析日志级别
	level := new(zapcore.Level)
	err = level.UnmarshalText([]byte(loggercfg.Level))
	if err != nil {
		return err
	}

	// 4. 创建核心 Core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 5. 生成 Logger，zap.AddCaller() 让输出附带调用者的文件名和行号
	log := zap.New(core, zap.AddCaller())

	// 6. 替换 zap 库中全局的 logger，后续在任意位置可以使用 zap.L().Info(...) 输出日志
	zap.ReplaceGlobals(log)

	return nil
}

// getEncoder 配置日志的编码格式
func getEncoder() zapcore.Encoder {
	// 获取 Zap 内置的生产环境默认配置 (自带一些字段键名的标准化)
	encoderConfig := zap.NewProductionEncoderConfig()

	// ----------------- 常见调整 -----------------

	// 1. 时间格式：默认是不可读的 Unix 时间戳，改为 ISO8601 (人类可读，例如: 2023-10-25T12:00:00.000Z)
	encoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder

	// 2. 日志级别格式：大写输出 (例如 INFO, ERROR 代替 info, error)，视觉上更醒目
	encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder

	// 3. 函数调用信息：短路径格式 (例如 main.go:25 代替一长串的绝对路径)
	encoderConfig.EncodeCaller = zapcore.ShortCallerEncoder

	// 返回 JSON 格式的编码器。
	// 生产环境极力推荐 JSON，因为极其方便 Logstash、Fluentd 等日志采集工具进行解析。
	// 若是纯开发环境想在终端看彩色日志，可将其改为：zapcore.NewConsoleEncoder(encoderConfig)
	return zapcore.NewJSONEncoder(encoderConfig)
}

// getLogWriter 配置日志文件的写入逻辑与自动切割
func getLogWriter(filename string, maxSize, maxBackup, maxAge int) zapcore.WriteSyncer {
	// 引入 lumberjack 库来实现基于文件大小和时间的日志切割
	lumberJackLogger := &lumberjack.Logger{
		Filename:   filename,  // 日志文件的全路径位置 (如 "./logs/app.log")
		MaxSize:    maxSize,   // 文件触发切割的最大大小（单位：MB）
		MaxBackups: maxBackup, // 系统保留的旧日志文件最大个数
		MaxAge:     maxAge,    // 旧日志文件保留的最大天数
		Compress:   false,     // 是否使用 gzip 压缩旧文件 (如果服务器 CPU 资源紧张可设为 false，推荐通过定时任务转移日志)
	}

	return zapcore.NewMultiWriteSyncer(zapcore.AddSync(lumberJackLogger), zapcore.AddSync(os.Stdout))

}
