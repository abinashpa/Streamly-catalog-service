package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type CacheManager struct {
	client *redis.Client
}

func NewCacheManager(client *redis.Client) *CacheManager {
	return &CacheManager{client: client}
}

func (cm *CacheManager) Get(ctx context.Context, key string, target interface{}) bool {
	if cm.client == nil {
		return false
	}
	val, err := cm.client.Get(ctx, key).Result()
	if err != nil {
		return false
	}
	return json.Unmarshal([]byte(val), target) == nil
}

func (cm *CacheManager) Set(ctx context.Context, key string, value interface{}, expiration time.Duration) {
	if cm.client == nil {
		return
	}
	bytes, err := json.Marshal(value)
	if err == nil {
		_ = cm.client.Set(ctx, key, bytes, expiration).Err()
	}
}

func (cm *CacheManager) Invalidate(ctx context.Context, pattern string) {
	if cm.client == nil {
		return
	}
	keys, err := cm.client.Keys(ctx, pattern).Result()
	if err == nil && len(keys) > 0 {
		_ = cm.client.Del(ctx, keys...).Err()
	}
}

func (cm *CacheManager) IncrementView(ctx context.Context, catalogID string) (int64, error) {
	if cm.client == nil {
		return 0, fmt.Errorf("redis client unavailable")
	}
	key := fmt.Sprintf("views:%s", catalogID)
	return cm.client.Incr(ctx, key).Result()
}
