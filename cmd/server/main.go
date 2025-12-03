package main

import (
	"codec-svr/internal/config"
	"codec-svr/internal/link"
	"codec-svr/internal/observability"
	"codec-svr/internal/server"
	"codec-svr/internal/store"
)

func main() {
	cfg := config.Load()
	logger := observability.NewLogger()
	logger.Info("Starting codec-svr...", "port", cfg.TCPPort)

	// Inicializar Redis antes del server
	if err := store.InitRedis("localhost:6379", 0); err != nil {
		logger.Error("Redis init failed", "error", err)
		return
	}
	link.Init(cfg.ProxyAddr, logger)

	go observability.StartMetricsServer(cfg.MetricsPort)

	// Inicia el servidor TCP directamente
	if err := server.Start(":" + cfg.TCPPort); err != nil {
		logger.Error("TCP server failed", "error", err)
	}
}
