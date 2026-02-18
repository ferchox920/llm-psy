package service

import (
	"context"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const redisOTPAllowScript = `
local current = redis.call("INCR", KEYS[1])
if current == 1 then
  redis.call("EXPIRE", KEYS[1], ARGV[1])
end
return current
`

type redisOTPRateLimiter struct {
	client redisEvaler
	window time.Duration
	max    int
	prefix string
}

type redisEvaler interface {
	Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd
}

func NewRedisOTPRateLimiter(client *redis.Client, window time.Duration, max int) OTPRateLimiter {
	if client == nil {
		return nil
	}
	if window <= 0 {
		window = time.Minute
	}
	if max <= 0 {
		max = 1
	}
	return &redisOTPRateLimiter{
		client: client,
		window: window,
		max:    max,
		prefix: "otp:rl:",
	}
}

func (l *redisOTPRateLimiter) Allow(key string) bool {
	if l == nil || l.client == nil {
		return true
	}
	normalizedKey := strings.ToLower(strings.TrimSpace(key))
	if normalizedKey == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	redisKey := l.prefix + normalizedKey
	seconds := int(l.window.Seconds())
	if seconds <= 0 {
		seconds = 60
	}
	count, err := l.client.Eval(ctx, redisOTPAllowScript, []string{redisKey}, seconds).Int()
	if err != nil {
		return true
	}
	return count <= l.max
}
