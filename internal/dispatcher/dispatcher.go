package dispatcher

import (
	"encoding/hex"
	"fmt"
	"net"
	"sync"

	"codec-svr/internal/codec"
)

// --- Mapa global de dispositivos conectados ---
var (
	deviceRegistry = make(map[string]net.Conn)
	registryLock   sync.Mutex
)

// Register almacena el IMEI y la conexión TCP asociada
func Register(imei string, conn net.Conn) {
	registryLock.Lock()
	defer registryLock.Unlock()

	deviceRegistry[imei] = conn
	fmt.Printf("\033[36m[REGISTER]\033[0m Device IMEI=%s registered (%s)\n", imei, conn.RemoteAddr())
}

// Unregister elimina la conexión asociada a un IMEI (opcional, para limpieza)
func Unregister(imei string) {
	registryLock.Lock()
	defer registryLock.Unlock()

	if _, exists := deviceRegistry[imei]; exists {
		delete(deviceRegistry, imei)
		fmt.Printf("\033[35m[UNREGISTER]\033[0m Device IMEI=%s removed from registry\n", imei)
	}
}

// ProcessIncoming maneja los datos AVL entrantes
func ProcessIncoming(conn net.Conn, data []byte) {
	rawHex := hex.EncodeToString(data)
	fmt.Printf("\033[33m[WARN]\033[0m RAW HEX (%d bytes): %s\n", len(data), rawHex)

	parsed, err := codec.ParseCodec8E(data)
	if err != nil {
		fmt.Printf("\033[31m[ERROR]\033[0m error parsing data: %v\n", err)
		return
	}

	// Mostrar los datos relevantes
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data → Lat: %.6f, Lon: %.6f, Ign: %v, Batt: %v, ExtVolt: %v\n",
		parsed["latitude"], parsed["longitude"], parsed["ignition"], parsed["battery_level"], parsed["external_voltage_mv"])

	// (en futuro aquí podemos enviar por gRPC, guardar en DB, etc.)
}
