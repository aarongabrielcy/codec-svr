package tcp

import (
	"bufio"
	"encoding/hex"
	"fmt"
	"net"
	"strings"
	"time"

	"codec-svr/internal/codec"
	"codec-svr/internal/dispatcher"
)

func Start(addr string, handler func(net.Conn)) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	defer ln.Close()
	fmt.Println("TCP server listening on", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("accept error:", err)
			continue
		}
		conn.SetDeadline(time.Now().Add(5 * time.Minute))
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()
	fmt.Println("New connection from:", conn.RemoteAddr())

	reader := bufio.NewReader(conn)
	buf := make([]byte, 1024)

	for {
		n, err := reader.Read(buf)
		if err != nil {
			fmt.Println("connection closed or read error:", err)
			return
		}

		data := buf[:n]
		fmt.Printf("Received %d bytes from %s\n", n, conn.RemoteAddr())
		fmt.Printf("[WARN] RAW HEX (%d bytes): %s\n", n, strings.ToUpper(hex.EncodeToString(data)))

		// Si son 17 bytes y los primeros dos son longitud → es IMEI handshake
		if n == 17 && data[0] == 0x00 && data[1] == 0x0F {
			imei := string(data[2:17])
			fmt.Printf("\033[36m[HANDSHAKE]\033[0m IMEI detected: %s\n", imei)
			// ACK al dispositivo
			conn.Write([]byte{0x01})
			dispatcher.Register(imei, conn)
			continue
		}

		// Si es paquete con preámbulo 0x00000000 → es AVL Data
		if len(data) > 8 && data[0] == 0x00 && data[1] == 0x00 && data[2] == 0x00 && data[3] == 0x00 {
			fmt.Println("\033[33m[AVL]\033[0m Codec8E data packet detected")

			parsed, err := codec.ParseCodec8E(data)
			if err != nil {
				fmt.Printf("[ERROR] parsing data: %v\n", err)
			} else {
				fmt.Printf("[INFO] Parsed AVL OK: %+v\n", parsed)
			}
			continue
		}

		// Si no es ninguno, solo muestra el contenido
		fmt.Println("[INFO] Unrecognized packet type, raw data logged.")
	}
}
