package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

type mockRedisKVClient struct {
	lastSetKey string
	lastSetVal interface{}
	lastSetTTL time.Duration
	lastExists []string
	lastDel    []string

	setErr    error
	existsErr error
	delErr    error
	existsN   int64
}

func (m *mockRedisKVClient) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	m.lastSetKey = key
	m.lastSetVal = value
	m.lastSetTTL = expiration
	cmd := redis.NewStatusCmd(ctx)
	if m.setErr != nil {
		cmd.SetErr(m.setErr)
		return cmd
	}
	cmd.SetVal("OK")
	return cmd
}

func (m *mockRedisKVClient) Exists(ctx context.Context, keys ...string) *redis.IntCmd {
	m.lastExists = keys
	cmd := redis.NewIntCmd(ctx)
	if m.existsErr != nil {
		cmd.SetErr(m.existsErr)
		return cmd
	}
	cmd.SetVal(m.existsN)
	return cmd
}

func (m *mockRedisKVClient) Del(ctx context.Context, keys ...string) *redis.IntCmd {
	m.lastDel = keys
	cmd := redis.NewIntCmd(ctx)
	if m.delErr != nil {
		cmd.SetErr(m.delErr)
		return cmd
	}
	cmd.SetVal(1)
	return cmd
}

func TestMemoryRefreshTokenStore_Basics(t *testing.T) {
	store := NewMemoryRefreshTokenStore()

	ok, err := store.Exists("missing")
	if err != nil || ok {
		t.Fatalf("expected missing token false,nil; got %v,%v", ok, err)
	}

	if err := store.Store("jti-1", "u1", 50*time.Millisecond); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	ok, err = store.Exists("jti-1")
	if err != nil || !ok {
		t.Fatalf("expected token exists, got %v,%v", ok, err)
	}

	time.Sleep(70 * time.Millisecond)
	ok, err = store.Exists("jti-1")
	if err != nil || ok {
		t.Fatalf("expected token expired, got %v,%v", ok, err)
	}
}

func TestMemoryRefreshTokenStore_RevokeAndEmptyJTI(t *testing.T) {
	store := NewMemoryRefreshTokenStore()
	if err := store.Store("", "u1", time.Minute); err != nil {
		t.Fatalf("empty jti store should be no-op, got %v", err)
	}
	if err := store.Store("jti-2", "u1", time.Minute); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if err := store.Revoke("jti-2"); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	ok, err := store.Exists("jti-2")
	if err != nil || ok {
		t.Fatalf("expected revoked token absent, got %v,%v", ok, err)
	}
}

func TestRedisRefreshTokenStore_Basics(t *testing.T) {
	mock := &mockRedisKVClient{existsN: 1}
	store := &redisRefreshTokenStore{
		client: mock,
		prefix: "auth:refresh:",
	}

	if err := store.Store(" j1 ", "u1", 0); err != nil {
		t.Fatalf("store failed: %v", err)
	}
	if mock.lastSetKey != "auth:refresh:j1" {
		t.Fatalf("unexpected key, got %q", mock.lastSetKey)
	}
	if mock.lastSetTTL <= 0 {
		t.Fatalf("expected positive TTL fallback, got %v", mock.lastSetTTL)
	}

	ok, err := store.Exists(" j1 ")
	if err != nil || !ok {
		t.Fatalf("expected exists true,nil; got %v,%v", ok, err)
	}
	if len(mock.lastExists) != 1 || mock.lastExists[0] != "auth:refresh:j1" {
		t.Fatalf("unexpected exists key: %+v", mock.lastExists)
	}

	if err := store.Revoke(" j1 "); err != nil {
		t.Fatalf("revoke failed: %v", err)
	}
	if len(mock.lastDel) != 1 || mock.lastDel[0] != "auth:refresh:j1" {
		t.Fatalf("unexpected del key: %+v", mock.lastDel)
	}
}

func TestRedisRefreshTokenStore_ErrorPathsAndEmptyJTI(t *testing.T) {
	mock := &mockRedisKVClient{
		setErr:    errors.New("set failed"),
		existsErr: errors.New("exists failed"),
		delErr:    errors.New("del failed"),
	}
	store := &redisRefreshTokenStore{
		client: mock,
		prefix: "auth:refresh:",
	}

	if err := store.Store("", "u1", time.Minute); err != nil {
		t.Fatalf("empty jti store should be no-op, got %v", err)
	}
	ok, err := store.Exists("")
	if err != nil || ok {
		t.Fatalf("empty jti exists should be false,nil; got %v,%v", ok, err)
	}
	if err := store.Revoke(""); err != nil {
		t.Fatalf("empty jti revoke should be no-op, got %v", err)
	}

	if err := store.Store("j2", "u1", time.Minute); err == nil {
		t.Fatalf("expected store error")
	}
	if _, err := store.Exists("j2"); err == nil {
		t.Fatalf("expected exists error")
	}
	if err := store.Revoke("j2"); err == nil {
		t.Fatalf("expected revoke error")
	}
}
