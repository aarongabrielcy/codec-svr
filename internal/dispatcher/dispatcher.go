package dispatcher

import (
	"encoding/hex"
	"fmt"
	"net"
	"sync"

	"codec-svr/internal/codec"
	"codec-svr/internal/store"
)

var previousStates = make(map[string]map[string]int)

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
	fmt.Printf("[INFO] Parsed AVL OK: %+v\n", parsed)

	// 🔹 Obtener valores clave
	io := parsed["io"].(map[int]map[string]any)
	imei := fmt.Sprintf("%v", parsed["imei"])

	ign := safeIOVal(io, 239)
	bat := safeIOVal(io, 113)
	extVolt := safeIOVal(io, 66)
	out1 := safeIOVal(io, 240)
	out2 := safeIOVal(io, 237)

	fmt.Printf("[STATE] IMEI:%s Ign:%d Batt:%d%% ExtVolt:%d Out1:%d Out2:%d\n", imei, ign, bat, extVolt, out1, out2)
	// 🔹 Detectar cambios de estado
	updateIfChanged(imei, "ign", ign)
	updateIfChanged(imei, "out1", out1)
	updateIfChanged(imei, "out2", out2)
	updateIfChanged(imei, "bat", bat)
	// Mostrar los datos relevantes
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data → Lat: %.6f, Lon: %.6f, Ign: %v, Batt: %v, ExtVolt: %v\n",
		parsed["latitude"], parsed["longitude"], parsed["ignition"], parsed["battery_level"], parsed["external_voltage_mv"])

	// (en futuro aquí podemos enviar por gRPC, guardar en DB, etc.)
}

func safeIOVal(io map[int]map[string]any, id int) int {
	if v, ok := io[id]; ok {
		if val, ok := v["val"].(int); ok {
			return val
		}
	}
	return 0
}

func updateIfChanged(imei, key string, newVal int) {
	if previousStates[imei] == nil {
		previousStates[imei] = make(map[string]int)
	}
	oldVal := previousStates[imei][key]
	if oldVal != newVal {
		fmt.Printf("[EVENT] Cambio detectado %s → %s: %d -> %d\n", imei, key, oldVal, newVal)
		previousStates[imei][key] = newVal
		// 🔹 Guardar evento en Redis
		store.SaveEventRedis(fmt.Sprintf("%s:%s", imei, key), newVal)
	}
}
