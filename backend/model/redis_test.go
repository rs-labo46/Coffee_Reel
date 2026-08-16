package model

import (
	"context"
	"testing"

	"coffee-reel/entity"
)

func TestNewRedisRejectsInvalidConfiguration(t *testing.T) {
	tests := []struct {
		name   string
		ctx    context.Context
		config entity.RedisConfig
	}{
		{name: "missing context", config: entity.RedisConfig{Host: "localhost", Port: "6379"}},
		{name: "missing host", ctx: context.Background(), config: entity.RedisConfig{Port: "6379"}},
		{name: "missing port", ctx: context.Background(), config: entity.RedisConfig{Host: "localhost"}},
		{name: "negative database", ctx: context.Background(), config: entity.RedisConfig{Host: "localhost", Port: "6379", DB: -1}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := NewRedis(tt.ctx, tt.config); err == nil {
				t.Fatal("NewRedis() error = nil")
			}
		})
	}
}
