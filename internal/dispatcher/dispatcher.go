package dispatcher

import (
	"encoding/hex"
	"fmt"
	"runtime/debug"
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

	// usar getValAny en lugar de getVal
	ign := getValAny(ioMap, 239)
	extVolt := getValAny(ioMap, 66)
	battery := getValAny(ioMap, 113)
	in1 := getValAny(ioMap, 200)
	out1 := getValAny(ioMap, 237)
	movement := getValAny(ioMap, 240)

	fmt.Printf("[STATE] IMEI=%s Ignition=%d Battery=%d ExtVolt=%d In1=%d Out1=%d Move=%d\n",
		imei, ign, battery, extVolt, in1, out1, movement)

	emitIfChanged(imei, "ign", ign)
	emitIfChanged(imei, "out1", out1)
	emitIfChanged(imei, "in1", in1)
	emitIfChanged(imei, "bat", battery)
	emitIfChanged(imei, "extvolt", extVolt)

	_ = time.Now()
}

// función robusta que maneja cualquier tipo numérico del mapa IO
func getValAny(ioMap map[int]map[string]interface{}, id int) int {
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
