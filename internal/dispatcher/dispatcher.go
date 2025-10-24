package dispatcher

import (
	"fmt"
	"net"
	"sync"
)

var connections = struct {
	sync.RWMutex
	data map[string]net.Conn
}{data: make(map[string]net.Conn)}

func Register(imei string, conn net.Conn) {
	connections.Lock()
	connections.data[imei] = conn
	connections.Unlock()
}

func SendCommand(imei string, cmd []byte) error {
	connections.RLock()
	conn, ok := connections.data[imei]
	connections.RUnlock()
	if !ok {
		return fmt.Errorf("device %s not connected", imei)
	}
	_, err := conn.Write(cmd)
	return err
}
