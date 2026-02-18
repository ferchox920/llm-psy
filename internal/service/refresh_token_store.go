package service

import (
	"context"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// RefreshTokenStore guarda jti para refresh tokens y permite revocarlos.
type RefreshTokenStore interface {
	Store(jti, userID string, ttl time.Duration) error
	Exists(jti string) (bool, error)
	Revoke(jti string) error
}

type memoryRefreshTokenStore struct {
	mu    sync.Mutex
	items map[string]time.Time
}

func NewMemoryRefreshTokenStore() RefreshTokenStore {
	return &memoryRefreshTokenStore{
		items: make(map[string]time.Time),
	}
}

func (s *memoryRefreshTokenStore) Store(jti, _ string, ttl time.Duration) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if strings.TrimSpace(jti) == "" {
		return nil
	}
	s.items[jti] = time.Now().UTC().Add(ttl)
	return nil
}

func (s *memoryRefreshTokenStore) Exists(jti string) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	exp, ok := s.items[jti]
	if !ok {
		return false, nil
	}
	if time.Now().UTC().After(exp) {
		delete(s.items, jti)
		return false, nil
	}
	return true, nil
}

func (s *memoryRefreshTokenStore) Revoke(jti string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, jti)
	return nil
}

type redisRefreshTokenStore struct {
	client redisKVClient
	prefix string
}

type redisKVClient interface {
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

func NewRedisRefreshTokenStore(client *redis.Client) RefreshTokenStore {
	if client == nil {
		return nil
	}
	return &redisRefreshTokenStore{
		client: client,
		prefix: "auth:refresh:",
	}
}

func (s *redisRefreshTokenStore) Store(jti, userID string, ttl time.Duration) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return nil
	}
	if ttl <= 0 {
		ttl = time.Minute
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return s.client.Set(ctx, s.prefix+jti, userID, ttl).Err()
}

func (s *redisRefreshTokenStore) Exists(jti string) (bool, error) {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	n, err := s.client.Exists(ctx, s.prefix+jti).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

func (s *redisRefreshTokenStore) Revoke(jti string) error {
	jti = strings.TrimSpace(jti)
	if jti == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	return s.client.Del(ctx, s.prefix+jti).Err()
}
