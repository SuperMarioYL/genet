package logger

import (
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

// GinLogger 返回 Gin 请求日志中间件
func GinLogger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		path := c.Request.URL.Path
		query := c.Request.URL.RawQuery

		// 处理请求
		c.Next()

		// 计算耗时
		latency := time.Since(start)

		// 获取用户信息
		username, _ := c.Get("username")
		usernameStr := ""
		if username != nil {
			usernameStr = username.(string)
		}

		// 构建日志字段
		fields := []zap.Field{
			zap.Int("status", c.Writer.Status()),
			zap.String("method", c.Request.Method),
			zap.String("path", path),
			zap.String("ip", c.ClientIP()),
			zap.Duration("latency", latency),
			zap.String("user-agent", c.Request.UserAgent()),
		}

		if query != "" {
			fields = append(fields, zap.String("query", query))
		}

		if usernameStr != "" {
			fields = append(fields, zap.String("user", usernameStr))
		}

		if len(c.Errors) > 0 {
			fields = append(fields, zap.String("errors", c.Errors.ByType(gin.ErrorTypePrivate).String()))
		}

		// 根据状态码选择日志级别
		status := c.Writer.Status()
		msg := "HTTP Request"

		if status >= 500 {
			L.Error(msg, fields...)
		} else if status >= 400 {
			L.Warn(msg, fields...)
		} else {
			L.Info(msg, fields...)
		}
	}
}

// GinRecovery 返回 Gin 恢复中间件
func GinRecovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if err := recover(); err != nil {
				L.Error("Panic recovered",
					zap.Any("error", err),
					zap.String("path", c.Request.URL.Path),
					zap.String("method", c.Request.Method),
					zap.String("ip", c.ClientIP()),
				)
				c.AbortWithStatus(500)
			}
		}()
		c.Next()
	}
}
