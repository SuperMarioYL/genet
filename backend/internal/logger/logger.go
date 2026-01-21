package logger

import (
	"os"
	"strings"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	// L 全局 Logger
	L *zap.Logger

	// S 全局 SugaredLogger（更方便的 API）
	S *zap.SugaredLogger
)

// Config 日志配置
type Config struct {
	Level      string // debug, info, warn, error
	Format     string // json, console
	OutputPath string // stdout, stderr, or file path
}

// Init 初始化全局 Logger
func Init(cfg *Config) error {
	if cfg == nil {
		cfg = &Config{
			Level:      "info",
			Format:     "console",
			OutputPath: "stdout",
		}
	}

	// 解析日志级别
	level := zapcore.InfoLevel
	switch strings.ToLower(cfg.Level) {
	case "debug":
		level = zapcore.DebugLevel
	case "info":
		level = zapcore.InfoLevel
	case "warn", "warning":
		level = zapcore.WarnLevel
	case "error":
		level = zapcore.ErrorLevel
	}

	// 配置编码器
	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		FunctionKey:    zapcore.OmitKey,
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalColorLevelEncoder, // 彩色级别
		EncodeTime:     zapcore.ISO8601TimeEncoder,       // ISO8601 时间格式
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// JSON 格式使用非彩色级别
	if strings.ToLower(cfg.Format) == "json" {
		encoderConfig.EncodeLevel = zapcore.CapitalLevelEncoder
	}

	// 选择编码器
	var encoder zapcore.Encoder
	if strings.ToLower(cfg.Format) == "json" {
		encoder = zapcore.NewJSONEncoder(encoderConfig)
	} else {
		encoder = zapcore.NewConsoleEncoder(encoderConfig)
	}

	// 配置输出
	var writeSyncer zapcore.WriteSyncer
	switch strings.ToLower(cfg.OutputPath) {
	case "stdout", "":
		writeSyncer = zapcore.AddSync(os.Stdout)
	case "stderr":
		writeSyncer = zapcore.AddSync(os.Stderr)
	default:
		file, err := os.OpenFile(cfg.OutputPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		if err != nil {
			return err
		}
		writeSyncer = zapcore.AddSync(file)
	}

	// 创建 Core
	core := zapcore.NewCore(encoder, writeSyncer, level)

	// 创建 Logger
	L = zap.New(core, zap.AddCaller(), zap.AddCallerSkip(0))
	S = L.Sugar()

	return nil
}

// Named 创建命名的子 Logger
func Named(name string) *zap.Logger {
	if L == nil {
		Init(nil)
	}
	return L.Named(name)
}

// With 创建带字段的 Logger
func With(fields ...zap.Field) *zap.Logger {
	if L == nil {
		Init(nil)
	}
	return L.With(fields...)
}

// Sync 刷新日志缓冲区
func Sync() {
	if L != nil {
		L.Sync()
	}
}

// Debug 记录 Debug 级别日志
func Debug(msg string, fields ...zap.Field) {
	if L == nil {
		Init(nil)
	}
	L.Debug(msg, fields...)
}

// Info 记录 Info 级别日志
func Info(msg string, fields ...zap.Field) {
	if L == nil {
		Init(nil)
	}
	L.Info(msg, fields...)
}

// Warn 记录 Warn 级别日志
func Warn(msg string, fields ...zap.Field) {
	if L == nil {
		Init(nil)
	}
	L.Warn(msg, fields...)
}

// Error 记录 Error 级别日志
func Error(msg string, fields ...zap.Field) {
	if L == nil {
		Init(nil)
	}
	L.Error(msg, fields...)
}

// Fatal 记录 Fatal 级别日志并退出
func Fatal(msg string, fields ...zap.Field) {
	if L == nil {
		Init(nil)
	}
	L.Fatal(msg, fields...)
}
