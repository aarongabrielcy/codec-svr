package store

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var ctx = context.Background()
var rdb *redis.Client

func InitRedis(addr string, db int) {
	rdb = redis.NewClient(&redis.Options{
		Addr: addr,
		DB:   db,
	})
}

func SaveEventRedis(key string, value int) {
	if rdb == nil {
		fmt.Println("[WARN] Redis client not initialized")
		return
	}
	err := rdb.Set(ctx, key, value, 10*time.Minute).Err()
	if err != nil {
		fmt.Printf("[ERROR] Redis SET %s → %v\n", key, err)
	}
}
