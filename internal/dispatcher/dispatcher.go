package dispatcher

import (
	"encoding/hex"
	"fmt"
	"runtime/debug"
	"sort"
	"time"

	"codec-svr/internal/codec"
	"codec-svr/internal/store"
)

var previousStates = make(map[string]map[string]int)

func ProcessIncoming(imei string, data []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVER] %v\n%s\n", r, string(debug.Stack()))
		}
	}()

	rawHex := hex.EncodeToString(data)
	fmt.Printf("\033[33m[WARN]\033[0m RAW HEX (%d bytes): %s\n", len(data), rawHex)

	parsed, err := codec.ParseCodec8E(imei, data)
	if err != nil {
		fmt.Printf("[ERROR] error parsing data: %v\n", err)
		return
	}

	fmt.Printf("[INFO] Parsed AVL OK: %+v\n", parsed)

	// Mapa de IOs
	ioMap := parsed.IO
	if len(ioMap) == 0 {
		fmt.Println("[WARN] no IO elements found in parsed data")
		return
	}

	// === BLOQUE DE DEPURACIÓN: Mostrar IOs ordenados ===
	fmt.Println("─────────────────────────────")
	fmt.Printf("[DEBUG] IO MAP for IMEI %s\n", imei)
	ids := make([]int, 0, len(ioMap))
	for id := range ioMap {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	for _, id := range ids {
		fmt.Printf("  • ID=%d → %v\n", id, ioMap[id])
	}
	fmt.Println("─────────────────────────────")

	// === Asignación de variables de estado ===
	ign := getValAny(ioMap, 239, 66, 69, 78)      // Ignition
	battery := getValAny(ioMap, 67, 10, 240, 241) // Internal battery voltage
	extVolt := getValAny(ioMap, 21, 68, 70, 72)   // External voltage / power supply
	in1 := getValAny(ioMap, 1, 200)               // Digital input 1
	in2 := getValAny(ioMap, 2, 201)               // Digital input 2
	out1 := getValAny(ioMap, 179, 178, 237)       // Digital output 1
	movement := getValAny(ioMap, 240, 181, 182)   // Movement or motion
	ain1 := getValAny(ioMap, 9, 205, 206)         // Analog input 1 (ADC1)

	fmt.Printf("[DEBUG] IO MAP DUMP: %+v\n", ioMap)
	fmt.Printf("[STATE] IMEI=%s Ignition=%d Battery=%d ExtVolt=%d In1=%d In2=%d Out1=%d AIn1=%d Move=%d\n",
		imei, ign, battery, extVolt, in1, in2, out1, ain1, movement)

	// Emitir eventos solo si hubo cambio de valor
	emitIfChanged(imei, "ign", ign)
	emitIfChanged(imei, "bat", battery)
	emitIfChanged(imei, "extvolt", extVolt)
	emitIfChanged(imei, "in1", in1)
	emitIfChanged(imei, "in2", in2)
	emitIfChanged(imei, "out1", out1)
	emitIfChanged(imei, "ain1", ain1)
	emitIfChanged(imei, "move", movement)

	_ = time.Now()
}

func getValAny(ioMap map[int]int, ids ...int) int {
	for _, id := range ids {
		if val, exists := ioMap[id]; exists {
			return val
		}
	}
	return 0
}

func emitIfChanged(imei, key string, newVal int) {
	if previousStates[imei] == nil {
		previousStates[imei] = make(map[string]int)
	}
	oldVal := previousStates[imei][key]
	if oldVal != newVal {
		fmt.Printf("[EVENT] %s %s changed %d -> %d\n", imei, key, oldVal, newVal)
		previousStates[imei][key] = newVal

		store.SaveEventRedis(fmt.Sprintf("state:%s:%s", imei, key), newVal)
	}
}
