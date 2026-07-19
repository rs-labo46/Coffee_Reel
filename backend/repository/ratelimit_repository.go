package repository

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

type IRateLimitRepository interface {
	Allow(ctx context.Context, key string, rate, capacity, cost float64, nowMS, ttlMS int64) (allowed bool, remaining float64, retryAfterMS int64, err error)
}

type rateLimitRepository struct {
	rdb    *redis.Client
	script *redis.Script
}

func NewRateLimitRepository(rdb *redis.Client) IRateLimitRepository {
	lua := `
local key = KEYS[1]
local rate = tonumber(ARGV[1])
local capacity = tonumber(ARGV[2])
local cost = tonumber(ARGV[3])
local now_ms = tonumber(ARGV[4])
local ttl_ms = tonumber(ARGV[5])

if rate == nil or capacity == nil or cost == nil or now_ms == nil or ttl_ms == nil then
  return redis.error_reply('invalid rate limit arguments')
end

if rate <= 0 or capacity <= 0 or cost <= 0 or ttl_ms <= 0 then
  return redis.error_reply('invalid rate limit arguments')
end

local values = redis.call('HMGET',key,'tokens','updated_at')

local tokens = tonumber(values[1])
local updated_at = tonumber(values[2])

if tokens == nil then
  tokens = capacity
end

if updated_at == nil then
  updated_at = now_ms
end

if now_ms < updated_at then
  now_ms = updated_at
end

local elapsed_ms = now_ms - updated_at
local refill = (elapsed_ms / 1000.0) * rate

tokens = math.min(capacity,tokens + refill)

local allowed = 0
local retry_after_ms = 0

if tokens >= cost then
  allowed = 1
  tokens = tokens - cost
else
  local needed = cost - tokens
  retry_after_ms = math.ceil(
    (needed / rate) * 1000.0
  )
end

redis.call('HSET',key,'tokens',tostring(tokens),'updated_at',now_ms)

redis.call('PEXPIRE',key,ttl_ms)

return {tostring(allowed),tostring(tokens),tostring(retry_after_ms)}
`

	return &rateLimitRepository{rdb: rdb, script: redis.NewScript(lua)}
}

func (r *rateLimitRepository) Allow(ctx context.Context, key string, rate, capacity, cost float64, nowMS, ttlMS int64) (allowed bool, remaining float64, retryAfterMS int64, err error) {
	if r.rdb == nil {
		return false, 0, 0, fmt.Errorf("redis client is required")
	}
	if key == "" {
		return false, 0, 0, fmt.Errorf("rate limit key is required")
	}
	if rate <= 0 || capacity <= 0 || cost <= 0 || nowMS <= 0 || ttlMS <= 0 {
		return false, 0, 0, fmt.Errorf("invalid rate limit arguments")
	}

	values, err := r.script.Run(ctx, r.rdb, []string{key}, rate, capacity, cost, nowMS, ttlMS).Float64Slice()

	if err != nil {
		return false, 0, 0, fmt.Errorf("evaluate rate limit: %w", err)
	}

	if len(values) != 3 {
		return false, 0, 0, fmt.Errorf("evaluate rate limit: invalid result")
	}
	return values[0] == 1, values[1], int64(values[2]), nil
}
