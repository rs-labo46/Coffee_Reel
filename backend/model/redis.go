package model

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"coffee-reel/entity"

	"github.com/redis/go-redis/v9"
)

func NewRedis(ctx context.Context, config entity.RedisConfig) (*redis.Client, error) {
	if ctx == nil {
		return nil, fmt.Errorf("redis context is required")
	}
	if strings.TrimSpace(config.Host) == "" || strings.TrimSpace(config.Port) == "" || config.DB < 0 {
		return nil, fmt.Errorf("redis configuration is invalid")
	}

	client := redis.NewClient(&redis.Options{
		Addr:         net.JoinHostPort(config.Host, config.Port),
		Password:     config.Password,
		DB:           config.DB,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  250 * time.Millisecond,
		WriteTimeout: 250 * time.Millisecond,
		PoolSize:     50,
		MinIdleConns: 5,
	})

	pingContext, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	if err := client.Ping(pingContext).Err(); err != nil {
		connectErr := fmt.Errorf("connect to redis: %w", err)
		if closeErr := client.Close(); closeErr != nil {
			return nil, errors.Join(connectErr, fmt.Errorf("close redis after connection failure: %w", closeErr))
		}
		return nil, connectErr
	}

	return client, nil
}
