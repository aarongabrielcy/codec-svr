package main

import (
	"codec-svr/internal/config"
	"codec-svr/internal/observability"
	tcp "codec-svr/internal/server"
	"codec-svr/internal/store"
	"net"
)

func main() {
	cfg := config.Load()

	logger := observability.NewLogger()
	logger.Info("Starting codec-svr...", "port", cfg.TCPPort)

	go observability.StartMetricsServer(cfg.MetricsPort)
	server := &tcp.TcpServer{}

	err := tcp.Start(":"+cfg.TCPPort, func(conn net.Conn) {
		logger.Info("New connection from", "addr", conn.RemoteAddr())
		// Aquí llamarías a tu handler de conexión real
		store.InitRedis("localhost:6379", 0)
		go server.HandleConnection(conn)

	})
	if err != nil {
		logger.Error("TCP server failed", "error", err)
	}
}
