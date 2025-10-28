package dispatcher

import (
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime/debug"
	"sort"
	"time"

	"codec-svr/internal/codec"
	"codec-svr/internal/observability"
	"codec-svr/internal/pipeline"
	"codec-svr/internal/store"
)

var previousStates = make(map[string]map[string]int)

func ProcessIncoming(imei string, frame []byte) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Printf("[PANIC RECOVER] %v\n%s\n", r, string(debug.Stack()))
		}
	}()

	rawHex := hex.EncodeToString(frame)
	fmt.Printf("\033[33m[WARN]\033[0m RAW HEX (%d bytes): %s\n", len(frame), rawHex)

	start := time.Now()
	parsed, err := codec.ParseCodec8E(frame)
	observability.ObserveParseLatency(start)
	if err != nil {
		observability.ParseErrors.Inc()
		fmt.Printf("[ERROR] parsing data: %v\n", err)
		return
	}

	fmt.Printf("[INFO] Parsed AVL OK: ts=%v prio=%v lat=%.6f lon=%.6f\n",
		parsed["timestamp"], parsed["priority"], toFloat(parsed["latitude"]), toFloat(parsed["longitude"]))

	// 1) Normalizar IOs a map[int]int
	ioMap := extractIOIntMap(parsed["io"])
	if len(ioMap) == 0 {
		fmt.Println("[WARN] no IO elements found")
	}

	// 2) Debug ordenado
	ids := make([]int, 0, len(ioMap))
	for id := range ioMap {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	fmt.Println("─────────────────────────────")
	fmt.Printf("[DEBUG] IO MAP for IMEI %s\n", imei)
	for _, id := range ids {
		fmt.Printf("  • ID=%d → %d\n", id, ioMap[id])
	}
	fmt.Println("─────────────────────────────")

	// 3) Detectar cambios, persistir a Redis y llevar métricas
	setState := func(key string, val int) {
		if previousStates[imei] == nil {
			previousStates[imei] = make(map[string]int)
		}
		old := previousStates[imei][key]
		if old != val {
			fmt.Printf("[EVENT] %s %s changed %d -> %d\n", imei, key, old, val)
			previousStates[imei][key] = val
			store.SaveEventRedisSafe(fmt.Sprintf("state:%s:%s", imei, key), val)
			observability.IOChanges.WithLabelValues(key).Inc()
		}
	}

	// Mapea lo principal (ajustable a tu FMC125)
	in1 := getValAny(ioMap, 1, 200)
	in2 := getValAny(ioMap, 2, 201)
	ign := getValAny(ioMap, 239)
	move := getValAny(ioMap, 240)
	out1 := getValAny(ioMap, 179, 178, 237)
	batt := getValAny(ioMap, 67)     // %
	extmv := getValAny(ioMap, 66)    // mV
	ain1 := getValAny(ioMap, 9, 205) // raw

	setState("in1", in1)
	setState("in2", in2)
	setState("ign", ign)
	setState("move", move)
	setState("out1", out1)
	setState("bat", batt)
	setState("extvolt", extmv)
	setState("ain1", ain1)

	// 4) LEER DE REDIS los valores normalizados (fuente de verdad)
	keys := []string{
		fmt.Sprintf("state:%s:%s", imei, "in1"),
		fmt.Sprintf("state:%s:%s", imei, "in2"),
		fmt.Sprintf("state:%s:%s", imei, "ign"),
		fmt.Sprintf("state:%s:%s", imei, "move"),
		fmt.Sprintf("state:%s:%s", imei, "out1"),
		fmt.Sprintf("state:%s:%s", imei, "bat"),
		fmt.Sprintf("state:%s:%s", imei, "extvolt"),
		fmt.Sprintf("state:%s:%s", imei, "ain1"),
	}
	redisVals := store.GetStatesRedis(keys)
	// construir un map normalizado key->int
	state := map[string]int{
		"in1":     redisVals[keys[0]],
		"in2":     redisVals[keys[1]],
		"ign":     redisVals[keys[2]],
		"move":    redisVals[keys[3]],
		"out1":    redisVals[keys[4]],
		"bat":     redisVals[keys[5]],
		"extvolt": redisVals[keys[6]],
		"ain1":    redisVals[keys[7]],
	}

	// 5) Construir TrackingObject con GPS + estados de Redis y emitir por gRPC
	tr := pipeline.BuildTrackingFromStates(
		imei,
		parsed["timestamp"],
		toFloat(parsed["latitude"]),
		toFloat(parsed["longitude"]),
		int(toFloat(parsed["speed"])),
		int(toFloat(parsed["angle"])),
		int(toFloat(parsed["satellites"])),
		state,
	)

	msgs := pipeline.ToGRPC(tr)
	_ = msgs // aquí invocas tu cliente gRPC real
}

// ------------------------- helpers -------------------------

// Acepta distintos shapes que puede devolver codec.ParseCodec8E para "io" y lo convierte a map[int]int.
// Soporta:
//   - map[uint16]struct{ Size int; Val uint64 }  (campos exportados)
//   - map[string]map[string]uint64               (e.g., {"size":1,"val":5})
//   - map[string]interface{} con "val"
//   - map[int]int / map[uint16]uint64 / etc.
func extractIOIntMap(ioAny interface{}) map[int]int {
	res := make(map[int]int)
	if ioAny == nil {
		return res
	}
	rv := reflect.ValueOf(ioAny)
	if rv.Kind() != reflect.Map {
		return res
	}
	for _, mk := range rv.MapKeys() {
		key := toInt(mk.Interface())

		mv := rv.MapIndex(mk)
		if !mv.IsValid() {
			continue
		}
		v := mv.Interface()

		switch t := v.(type) {
		case int:
			res[key] = t
		case int32:
			res[key] = int(t)
		case int64:
			res[key] = int(t)
		case uint8:
			res[key] = int(t)
		case uint16:
			res[key] = int(t)
		case uint32:
			res[key] = int(t)
		case uint64:
			res[key] = int(t)
		case map[string]uint64:
			if val, ok := t["val"]; ok {
				res[key] = int(val)
			}
		case map[string]interface{}:
			if val, ok := t["val"]; ok {
				res[key] = toInt(val)
			}
		default:
			// ¿struct con campo exportado "Val"? (p.ej., ioItem{Size int; Val uint64})
			mv := reflect.ValueOf(v)
			if mv.Kind() == reflect.Struct {
				f := mv.FieldByName("Val")
				if f.IsValid() && f.CanInterface() {
					res[key] = toInt(f.Interface())
				}
			}
		}
	}
	return res
}

func toInt(x interface{}) int {
	switch v := x.(type) {
	case int:
		return v
	case int8:
		return int(v)
	case int16:
		return int(v)
	case int32:
		return int(v)
	case int64:
		return int(v)
	case uint:
		return int(v)
	case uint8:
		return int(v)
	case uint16:
		return int(v)
	case uint32:
		return int(v)
	case uint64:
		return int(v)
	case string:
		// tratar de parsear números en string (no obligatorio aquí)
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	default:
		// soportar reflect.Value directo
		rv := reflect.ValueOf(x)
		if rv.Kind() == reflect.Uint || rv.Kind() == reflect.Uint64 || rv.Kind() == reflect.Uint32 || rv.Kind() == reflect.Uint16 || rv.Kind() == reflect.Uint8 {
			return int(rv.Uint())
		}
		if rv.Kind() == reflect.Int || rv.Kind() == reflect.Int64 || rv.Kind() == reflect.Int32 || rv.Kind() == reflect.Int16 || rv.Kind() == reflect.Int8 {
			return int(rv.Int())
		}
		return 0
	}
}

func toFloat(x interface{}) float64 {
	switch v := x.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	default:
		return 0
	}
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

		store.SaveEventRedisSafe(fmt.Sprintf("state:%s:%s", imei, key), newVal)
	}
}
