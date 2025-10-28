package store

import (
	"context"
	"fmt"
	"strconv"
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

func SaveEventRedisSafe(key string, value int) {
	if rdb == nil {
		fmt.Println("[WARN] redis not initialized")
		return
	}
	err := rdb.Set(ctx, key, value, 10*time.Minute).Err()
	if err != nil {
		fmt.Printf("[ERROR] redis SET %s: %v\n", key, err)
	}
}

func GetStateRedis(key string) (int, bool) {
	if rdb == nil {
		return 0, false
	}
	val, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return 0, false
	}
	n, _ := strconv.Atoi(val)
	return n, true
}
func GetStatesRedis(keys []string) map[string]int {
	out := make(map[string]int, len(keys))
	if rdb == nil || len(keys) == 0 {
		return out
	}
	vals, err := rdb.MGet(ctx, keys...).Result()
	if err != nil {
		return out
	}
	for i, v := range vals {
		if v == nil {
			continue
		}
		s, _ := v.(string)
		n, _ := strconv.Atoi(s)
		out[keys[i]] = n
	}
	return out
}
