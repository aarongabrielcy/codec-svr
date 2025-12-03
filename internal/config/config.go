package config

import (
	"os"
)

type Config struct {
	TCPPort           string
	MetricsPort       string
	GRPCServer        string
	RedisAddr         string
	GetVerOnHandshake bool
	ProxyAddr         string
}

func Load() Config {
	return Config{
		TCPPort:           getEnv("TCP_PORT", "8001"),
		MetricsPort:       getEnv("METRICS_PORT", "9000"),
		GRPCServer:        getEnv("GRPC_SERVER", "localhost:50051"),
		RedisAddr:         getEnv("REDIS_ADDR", "localhost:6379"),
		GetVerOnHandshake: getEnv("GETVER_ON_HANDSHAKE", "1") != "0",
		ProxyAddr:         getEnv("PROXY_ADDR", "201.122.135.23:6100"),
	}
}

func getEnv(key, fallback string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return fallback
}
