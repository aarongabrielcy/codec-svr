package server

import (
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"sync"
	"time"

	"codec-svr/internal/dispatcher"
	"codec-svr/internal/utilities"
)

// Estructura principal del servidor
type TcpServer struct{}

var (
	activeConnections sync.Map // IMEI -> net.Conn
)

// 🔹 NUEVO: función Start() compatible con tu main.go
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
			handler(c)              // callback del main.go
			srv.HandleConnection(c) // manejo interno (IMEI, AVL, etc.)
		}(conn)
	}
}

// Maneja una conexión TCP entrante
func (srv *TcpServer) HandleConnection(conn net.Conn) {
	defer conn.Close()

	var deviceIMEI string
	defer func() {
		if deviceIMEI != "" {
			activeConnections.Delete(deviceIMEI)
			log.Printf("[INFO] Dispositivo desconectado: %s", deviceIMEI)
		}
	}()

	err := SetTCPOptions(conn)
	if err != nil {
		log.Println("TCP options error: ", err)
	}

	netData := make([]byte, 2048)

	for {
		lenBytes, err := conn.Read(netData)
		if err != nil {
			if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
				continue
			} else if err != io.EOF {
				fmt.Println("Read error: ", err.Error())
				break
			} else if err == io.EOF {
				break
			}
		}

		if lenBytes == 0 {
			continue
		}

		message := strings.TrimSpace(string(netData[:lenBytes]))
		data := netData[:lenBytes]

		utilities.CreateLog("ALLTRACKINGS", message)

		// 🔹 Si aún no tenemos IMEI registrado, lo procesamos
		if deviceIMEI == "" && isIMEIMessage(message) {
			deviceIMEI = extractIMEI(message)
			if deviceIMEI != "" {
				activeConnections.Store(deviceIMEI, conn)
				log.Printf("[INFO] Nuevo dispositivo conectado: %s", deviceIMEI)
				_, _ = conn.Write([]byte{0x01}) // ACK
			} else {
				log.Printf("[WARN] IMEI inválido recibido desde %s", conn.RemoteAddr())
				conn.Write([]byte{0x00})
				conn.Close()
				return
			}
			continue
		}

		// 🔹 Procesar datos AVL
		go dispatcher.ProcessIncoming(conn, data)
	}
}

// Detecta si el mensaje es un IMEI (inicio de conexión)
func isIMEIMessage(data string) bool {
	data = strings.TrimSpace(data)
	return len(data) >= 15 && strings.IndexFunc(data, func(r rune) bool {
		return r < '0' || r > '9'
	}) == -1
}

// Extrae el IMEI de la cadena
func extractIMEI(data string) string {
	data = strings.TrimSpace(data)
	if len(data) >= 15 {
		return data[len(data)-15:]
	}
	return ""
}

func SetTCPOptions(conn net.Conn) error {
	switch conn := conn.(type) {
	case *net.TCPConn:
		if err := conn.SetLinger(0); err != nil {
			return err
		}
		if err := conn.SetNoDelay(false); err != nil {
			return err
		}
		if err := conn.SetKeepAlive(true); err != nil {
			return err
		}
		if err := conn.SetKeepAlivePeriod(60 * time.Second); err != nil {
			return err
		}
		return nil
	default:
		return fmt.Errorf("unknown connection type %T", conn)
	}
}
