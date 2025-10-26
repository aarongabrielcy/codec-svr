package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()
var rdb *redis.Client

func InitRedis(addr string, db int) error {
	rdb = redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})
	_, err := rdb.Ping(ctx).Result()
	if err != nil {
		return fmt.Errorf("redis ping failed: %w", err)
	}
	fmt.Println("[REDIS] connected")
	return nil
}

func SaveEventRedis(key string, value int) {
	if rdb == nil {
		fmt.Println("[WARN] redis not initialized")
		return
	}
	err := rdb.Set(ctx, key, value, 10*time.Minute).Err()
	if err != nil {
		fmt.Printf("[ERROR] redis SET %s: %v\n", key, err)
	}
}
