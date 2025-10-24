package tcp

import (
	"bufio"
	"fmt"
	"net"
	"time"

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
	buffer := make([]byte, 1024)

	for {
		n, err := reader.Read(buffer)
		if err != nil {
			fmt.Println("connection closed or read error:", err)
			return
		}

		data := buffer[:n]
		fmt.Printf("📦 Received %d bytes from %s\n", n, conn.RemoteAddr())
		dispatcher.ProcessIncoming(conn, data)
	}
}
