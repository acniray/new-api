package middleware

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/QuantumNous/new-api/common"

	"github.com/gin-gonic/gin"
)

const (
	SmsRateLimitMark      = "SMS"
	SmsMaxRequests        = 1  // 60秒内最多1次
	SmsRateLimitDuration  = 60 // 60秒时间窗口
)

func redisSmsRateLimiter(c *gin.Context) {
	ctx := context.Background()
	rdb := common.RDB
	key := "smsRate:" + SmsRateLimitMark + ":" + c.ClientIP()

	count, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		// fallback
		memorySmsRateLimiter(c)
		return
	}

	// 第一次设置键时设置过期时间
	if count == 1 {
		_ = rdb.Expire(ctx, key, time.Duration(SmsRateLimitDuration)*time.Second).Err()
	}

	// 检查是否超出限制
	if count <= int64(SmsMaxRequests) {
		c.Next()
		return
	}

	// 获取剩余等待时间
	ttl, err := rdb.TTL(ctx, key).Result()
	waitSeconds := int64(SmsRateLimitDuration)
	if err == nil && ttl > 0 {
		waitSeconds = int64(ttl.Seconds())
	}

	c.JSON(http.StatusTooManyRequests, gin.H{
		"success": false,
		"message": fmt.Sprintf("发送过于频繁，请等待 %d 秒后再试", waitSeconds),
	})
	c.Abort()
}

func memorySmsRateLimiter(c *gin.Context) {
	key := SmsRateLimitMark + ":" + c.ClientIP()

	if !inMemoryRateLimiter.Request(key, SmsMaxRequests, SmsRateLimitDuration) {
		c.JSON(http.StatusTooManyRequests, gin.H{
			"success": false,
			"message": "发送过于频繁，请稍后再试",
		})
		c.Abort()
		return
	}

	c.Next()
}

func SmsRateLimit() gin.HandlerFunc {
	return func(c *gin.Context) {
		if common.RedisEnabled {
			redisSmsRateLimiter(c)
		} else {
			inMemoryRateLimiter.Init(common.RateLimitKeyExpirationDuration)
			memorySmsRateLimiter(c)
		}
	}
}
