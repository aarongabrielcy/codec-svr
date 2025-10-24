package tcp

import (
	"fmt"
	"net"
	"time"
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
		conn.SetDeadline(time.Now().Add(2 * time.Minute))
		go handler(conn)
	}
}
