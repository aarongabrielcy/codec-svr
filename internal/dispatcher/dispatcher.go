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

	parsed, err := codec.ParseCodec8E(data)
	if err != nil {
		fmt.Printf("[ERROR] error parsing data: %v\n", err)
		return
	}

	fmt.Printf("[INFO] Parsed AVL OK: %+v\n", parsed)

	rawIO, ok := parsed["io"]
	if !ok {
		fmt.Println("[WARN] no IO elements found in parsed data")
		return
	}

	ioMap, ok := rawIO.(map[int]map[string]interface{})
	if !ok {
		fmt.Printf("[ERROR] unexpected 'io' type: %T\n", rawIO)
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
		val := ioMap[id]["val"]
		size := ioMap[id]["size"]
		fmt.Printf("  • ID=%d (size=%v) → %v\n", id, size, val)
	}
	fmt.Println("─────────────────────────────")
	ign := getValAny(ioMap, 239, 66, 69, 78)      // Ignition
	battery := getValAny(ioMap, 67, 10, 240, 241) // Internal battery voltage
	extVolt := getValAny(ioMap, 21, 68, 70, 72)   // External voltage / power supply
	in1 := getValAny(ioMap, 1, 9, 200)            // Digital input 1
	out1 := getValAny(ioMap, 179, 178, 237)       // Digital output 1
	movement := getValAny(ioMap, 240, 181, 182)   // Movement or motion

	fmt.Printf("[DEBUG] IO MAP DUMP: %+v\n", ioMap)
	fmt.Printf("[STATE] IMEI=%s Ignition=%d Battery=%d ExtVolt=%d In1=%d Out1=%d Move=%d\n",
		imei, ign, battery, extVolt, in1, out1, movement)

	emitIfChanged(imei, "ign", ign)
	emitIfChanged(imei, "out1", out1)
	emitIfChanged(imei, "in1", in1)
	emitIfChanged(imei, "bat", battery)
	emitIfChanged(imei, "extvolt", extVolt)

	_ = time.Now()
}

func getValAny(ioMap map[int]map[string]interface{}, ids ...int) int {
	for _, id := range ids {
		if v, exists := ioMap[id]; exists {
			val := v["val"]
			switch num := val.(type) {
			case int:
				return num
			case int32:
				return int(num)
			case int64:
				return int(num)
			case float32:
				return int(num)
			case float64:
				return int(num)
			case uint8:
				return int(num)
			case uint16:
				return int(num)
			case uint32:
				return int(num)
			case uint64:
				return int(num)
			default:
				fmt.Printf("[WARN] unexpected type for IO[%d]: %T → %v\n", id, val, val)
			}
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
