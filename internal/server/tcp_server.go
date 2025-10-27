package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"time"

	"codec-svr/internal/dispatcher"
	"codec-svr/internal/utilities"
)

type TcpServer struct{}

func Start(addr string, handler func(net.Conn)) error {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("error starting TCP server: %w", err)
	}
	defer listener.Close()

	log.Printf("[INFO] TCP Server listening on %s", addr)

	srv := &TcpServer{}

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Printf("[ERROR] accept error: %v", err)
			continue
		}

		go func(c net.Conn) {

			if handler != nil {
				handler(c)
			}

			srv.HandleConnection(c)
		}(conn)
	}
}

var activeConnections = make(map[string]net.Conn)

func (srv *TcpServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	var deviceIMEI string
	defer func() {
		if deviceIMEI != "" {
			delete(activeConnections, deviceIMEI)
			log.Printf("[INFO] Dispositivo desconectado: %s", deviceIMEI)
		}
	}()

	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetLinger(0)
		_ = tcpConn.SetNoDelay(false)
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(60 * time.Second)
	}

	buffer := make([]byte, 2048)
	for {
		n, err := conn.Read(buffer)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			}
			if err == io.EOF {
				return
			}
			log.Printf("[ERROR] read error: %v", err)
			return
		}
		if n == 0 {
			continue
		}

		data := make([]byte, n)
		copy(data, buffer[:n])

		utilities.CreateLog("ALLTRACKINGS", string(data))

		if deviceIMEI == "" && n == 17 && data[0] == 0x00 && data[1] == 0x0F {
			imei := string(data[2:17])
			deviceIMEI = imei
			activeConnections[imei] = conn
			log.Printf("[HANDSHAKE] IMEI detected: %s from %s", imei, conn.RemoteAddr())
			_, _ = conn.Write([]byte{0x01})
			continue
		}

		if deviceIMEI == "" {

			log.Printf("[WARN] packet received before IMEI registration (%d bytes) from %s", n, conn.RemoteAddr())
			continue
		}

		go dispatcher.ProcessIncoming(deviceIMEI, data)
	}
}
