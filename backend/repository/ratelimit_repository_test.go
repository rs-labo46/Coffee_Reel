//go:build integration

package repository

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestRateLimitRepositoryIntegrationTokenConsumptionRefillAndTTL(t *testing.T) {
	client, prefix := openRedisIntegrationClient(t)
	repository := NewRateLimitRepository(client)
	ctx := context.Background()
	key := prefix + "bucket"
	now := time.Now().UTC().UnixMilli()

	for i := 0; i < 3; i++ {
		allowed, remaining, retryAfter, err := repository.Allow(ctx, key, 0.1, 3, 1, now, 60000)
		if err != nil {
			t.Fatalf("Allow(%d) error = %v", i, err)
		}
		if !allowed || remaining != float64(2-i) || retryAfter != 0 {
			t.Fatalf("Allow(%d) = allowed:%v remaining:%v retry:%d", i, allowed, remaining, retryAfter)
		}
	}
	allowed, remaining, retryAfter, err := repository.Allow(ctx, key, 0.1, 3, 1, now, 60000)
	if err != nil {
		t.Fatalf("Allow(over limit) error = %v", err)
	}
	if allowed || remaining != 0 || retryAfter != 10000 {
		t.Fatalf("over limit = allowed:%v remaining:%v retry:%d", allowed, remaining, retryAfter)
	}

	allowed, remaining, retryAfter, err = repository.Allow(ctx, key, 0.1, 3, 1, now+5000, 60000)
	if err != nil {
		t.Fatalf("Allow(half refill) error = %v", err)
	}
	if allowed || remaining < 0.49 || remaining > 0.51 || retryAfter < 4999 || retryAfter > 5001 {
		t.Fatalf("half refill = allowed:%v remaining:%v retry:%d", allowed, remaining, retryAfter)
	}

	allowed, remaining, retryAfter, err = repository.Allow(ctx, key, 0.1, 3, 1, now+10000, 60000)
	if err != nil {
		t.Fatalf("Allow(full token refill) error = %v", err)
	}
	if !allowed || remaining < -0.001 || remaining > 0.001 || retryAfter != 0 {
		t.Fatalf("full refill = allowed:%v remaining:%v retry:%d", allowed, remaining, retryAfter)
	}

	ttl, err := client.PTTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("PTTL() error = %v", err)
	}
	if ttl <= 0 || ttl > 60*time.Second {
		t.Fatalf("TTL = %s, want (0, 60s]", ttl)
	}
}

func TestRateLimitRepositoryIntegrationConcurrentRequestsNeverExceedCapacity(t *testing.T) {
	client, prefix := openRedisIntegrationClient(t)
	repository := NewRateLimitRepository(client)
	ctx := context.Background()
	key := prefix + "concurrent"
	now := time.Now().UTC().UnixMilli()
	var allowedCount atomic.Int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	errorsCh := make(chan error, 50)

	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			allowed, _, _, err := repository.Allow(ctx, key, 0.1, 5, 1, now, 60000)
			if err != nil {
				errorsCh <- err
				return
			}
			if allowed {
				allowedCount.Add(1)
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errorsCh)
	for err := range errorsCh {
		t.Fatalf("concurrent Allow() error = %v", err)
	}
	if got := allowedCount.Load(); got != 5 {
		t.Fatalf("allowed requests = %d, want exactly capacity 5", got)
	}
}

func TestRateLimitRepositoryIntegrationRejectsInvalidArgumentsBeforeRedis(t *testing.T) {
	client, prefix := openRedisIntegrationClient(t)
	repository := NewRateLimitRepository(client)
	ctx := context.Background()
	now := time.Now().UTC().UnixMilli()

	tests := []struct {
		name     string
		key      string
		rate     float64
		capacity float64
		cost     float64
		nowMS    int64
		ttlMS    int64
	}{
		{name: "empty key", key: "", rate: 1, capacity: 1, cost: 1, nowMS: now, ttlMS: 1000},
		{name: "zero rate", key: prefix + "key", rate: 0, capacity: 1, cost: 1, nowMS: now, ttlMS: 1000},
		{name: "zero capacity", key: prefix + "key", rate: 1, capacity: 0, cost: 1, nowMS: now, ttlMS: 1000},
		{name: "zero cost", key: prefix + "key", rate: 1, capacity: 1, cost: 0, nowMS: now, ttlMS: 1000},
		{name: "zero time", key: prefix + "key", rate: 1, capacity: 1, cost: 1, nowMS: 0, ttlMS: 1000},
		{name: "zero ttl", key: prefix + "key", rate: 1, capacity: 1, cost: 1, nowMS: now, ttlMS: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, _, err := repository.Allow(ctx, tt.key, tt.rate, tt.capacity, tt.cost, tt.nowMS, tt.ttlMS); err == nil {
				t.Fatal("Allow() accepted invalid arguments")
			}
		})
	}
}
