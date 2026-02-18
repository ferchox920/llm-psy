package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type mockRedisEvaler struct {
	lastScript string
	lastKeys   []string
	lastArgs   []interface{}
	result     int64
	err        error
}

func (m *mockRedisEvaler) Eval(ctx context.Context, script string, keys []string, args ...interface{}) *redis.Cmd {
	m.lastScript = script
	m.lastKeys = keys
	m.lastArgs = args
	cmd := redis.NewCmd(ctx)
	if m.err != nil {
		cmd.SetErr(m.err)
		return cmd
	}
	cmd.SetVal(m.result)
	return cmd
}

func TestRedisOTPRateLimiterAllow(t *testing.T) {
	t.Run("nil receiver fail-open", func(t *testing.T) {
		var l *redisOTPRateLimiter
		if !l.Allow("user@example.com") {
			t.Fatalf("expected fail-open for nil limiter")
		}
	})

	t.Run("empty key rejected", func(t *testing.T) {
		l := &redisOTPRateLimiter{
			client: &mockRedisEvaler{result: 1},
			window: time.Minute,
			max:    3,
			prefix: "otp:rl:",
		}
		if l.Allow("   ") {
			t.Fatalf("expected empty key to be rejected")
		}
	})

	t.Run("allow when count within max", func(t *testing.T) {
		mock := &mockRedisEvaler{result: 2}
		l := &redisOTPRateLimiter{
			client: mock,
			window: 2 * time.Minute,
			max:    3,
			prefix: "otp:rl:",
		}
		if !l.Allow(" User@Example.com ") {
			t.Fatalf("expected allow when count <= max")
		}
		if len(mock.lastKeys) != 1 || mock.lastKeys[0] != "otp:rl:user@example.com" {
			t.Fatalf("unexpected key normalization, got %+v", mock.lastKeys)
		}
		if len(mock.lastArgs) != 1 || mock.lastArgs[0] != 120 {
			t.Fatalf("expected TTL seconds=120, got %+v", mock.lastArgs)
		}
		if mock.lastScript != redisOTPAllowScript {
			t.Fatalf("expected script to match")
		}
	})

	t.Run("deny when count exceeds max", func(t *testing.T) {
		l := &redisOTPRateLimiter{
			client: &mockRedisEvaler{result: 4},
			window: time.Minute,
			max:    3,
			prefix: "otp:rl:",
		}
		if l.Allow("user@example.com") {
			t.Fatalf("expected deny when count > max")
		}
	})

	t.Run("redis error fail-open", func(t *testing.T) {
		l := &redisOTPRateLimiter{
			client: &mockRedisEvaler{err: errors.New("redis down")},
			window: time.Minute,
			max:    3,
			prefix: "otp:rl:",
		}
		if !l.Allow("user@example.com") {
			t.Fatalf("expected fail-open on redis errors")
		}
	})
}
