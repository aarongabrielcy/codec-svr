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

func SaveEventRedisSafe(key string, value int, ttl ...time.Duration) {
	if rdb == nil {
		fmt.Println("[WARN] redis not initialized")
		return
	}
	expiration := time.Duration(0) // 0 = sin expiración
	if len(ttl) > 0 {
		expiration = ttl[0] // 10*time.Minute
	}

	err := rdb.Set(ctx, key, value, expiration).Err()
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

// al final del archivo
func SaveStringSafe(key, value string, ttl ...time.Duration) {
	if rdb == nil {
		fmt.Println("[WARN] redis not initialized")
		return
	}
	exp := time.Duration(0)
	if len(ttl) > 0 {
		exp = ttl[0]
	}
	if err := rdb.Set(ctx, key, value, exp).Err(); err != nil {
		fmt.Println("[REDIS] set error:", err)
	}
}

func GetStringSafe(key string) string {
	if rdb == nil {
		return ""
	}
	s, err := rdb.Get(ctx, key).Result()
	if err != nil {
		return ""
	}
	return s
}

func HSetPermIO(imei string, id uint16, val uint64) {
	key := imei
	field := strconv.Itoa(int(id))
	_ = rdb.HSet(ctx, key, field, strconv.FormatUint(val, 10)).Err()
}

// Obtiene el hash completo como map[string]uint64
func HGetAllPermIO(imei string) map[string]uint64 {
	key := imei
	out := map[string]uint64{}
	m, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return out
	}
	for k, v := range m {
		if n, err := strconv.ParseUint(v, 10, 64); err == nil {
			out[k] = n
		}
	}
	return out
}

// ---------------- Contador diario de comandos ----------------

// IncDailyCmdCounter incrementa un contador diario para un comando (por IMEI).
// Devuelve:
//
//	allowed = true  si todavía está por debajo o igual que max
//	allowed = false si ya excedió max (pero el contador igualmente subió)
//	current = valor actual del contador tras el INCR
func IncDailyCmdCounter(imei, cmd string, max int) (allowed bool, current int, err error) {
	if rdb == nil {
		return false, 0, fmt.Errorf("redis not initialized")
	}

	today := time.Now().Format("20060102") // YYYYMMDD
	key := fmt.Sprintf("dev:%s:cmd:%s:%s", imei, cmd, today)

	val, err := rdb.Incr(ctx, key).Result()
	if err != nil {
		return false, 0, err
	}

	// Primer uso del día → poner TTL para que caduque solo
	if val == 1 {
		_ = rdb.Expire(ctx, key, 48*time.Hour).Err()
	}

	if val > int64(max) {
		return false, int(val), nil
	}
	return true, int(val), nil
}
