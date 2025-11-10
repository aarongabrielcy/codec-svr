package dispatcher

import (
	"codec-svr/internal/codec"
	"codec-svr/internal/codec/fmc125"
	"codec-svr/internal/observability"
	"codec-svr/internal/pipeline"
	"codec-svr/internal/store"
	"encoding/hex"
	"fmt"
	"reflect"
	"runtime/debug"
	"sort"
	"strconv"
	"time"
)

var previousStates = make(map[string]map[string]int)

func ProcessIncoming(imei string, frame []byte) {
	isBatch := false
	if len(frame) >= 10 && (frame[8] == 0x08 || frame[8] == 0x8E) {
		isBatch = int(frame[9]) > 1 // Qty1 > 1
	}
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
	fmt.Printf("[INFO] Parsed AVL OK: codeid=%X ts=%v prio=%v lat=%.6f lon=%.6f alt=%d ang=%d spd=%d sat=%d\n", //%X convierte decimal a Hexadecimal
		parsed["codec_id"],
		parsed["timestamp"],
		parsed["priority"],
		toFloatAny(parsed["latitude"]),
		toFloatAny(parsed["longitude"]),
		toIntAny(parsed["altitude"]),
		toIntAny(parsed["angle"]),
		toIntAny(parsed["speed"]),
		toIntAny(parsed["satellites"]),
	)
	const bufferWindow = 120 * time.Second

	// determinar timestamp como time.Time (puede venir como time.Time o string RFC3339)
	var recTS time.Time
	if ts, ok := parsed["timestamp"].(time.Time); ok {
		recTS = ts
	} else if s, ok := parsed["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			recTS = t
		}
	}

	// msgType por batch/antigüedad
	msgType := "live"
	if isBatch {
		msgType = "buffer"
	} else if !recTS.IsZero() && time.Since(recTS) > bufferWindow {
		msgType = "buffer"
	}

	// obtener modelo y fw desde Redis (si ya los guardó HandleGetVerResponse)
	model := store.GetStringSafe("dev:" + imei + ":model")
	fwVer := store.GetStringSafe("dev:" + imei + ":fw")

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

	in1 := getValAny(ioMap, fmc125.DIn1)
	in2 := getValAny(ioMap, fmc125.DIn2)
	ign := getValAny(ioMap, fmc125.Ignition)
	move := getValAny(ioMap, fmc125.Movement)
	out1 := getValAny(ioMap, fmc125.DOut1)
	batt := getValAny(ioMap, fmc125.BatteryVolt)   // mV
	battPerc := getValAny(ioMap, fmc125.BattLevel) // %
	extmv := getValAny(ioMap, fmc125.ExtVolt)      // mV
	ain1 := getValAny(ioMap, fmc125.AIn1)          // raw
	sleep := getValAny(ioMap, fmc125.SleepMode)
	vehicleSpd := getValAny(ioMap, fmc125.VehicleSpeed)

	setState("in1", in1)
	setState("in2", in2)
	setState("ign", ign)
	setState("move", move)
	setState("out1", out1)
	setState("batVolt", batt)
	setState("batPerc", battPerc)
	setState("extvolt", extmv)
	setState("ain1", ain1)
	//setState("sleep", sleep)

	// 4) LEER DE REDIS los valores normalizados (fuente de verdad)
	keys := []string{
		fmt.Sprintf("state:%s:%s", imei, "in1"),
		fmt.Sprintf("state:%s:%s", imei, "in2"),
		fmt.Sprintf("state:%s:%s", imei, "ign"),
		fmt.Sprintf("state:%s:%s", imei, "move"),
		fmt.Sprintf("state:%s:%s", imei, "out1"),
		fmt.Sprintf("state:%s:%s", imei, "batVolt"),
		fmt.Sprintf("state:%s:%s", imei, "extvolt"),
		fmt.Sprintf("state:%s:%s", imei, "ain1"),
		fmt.Sprintf("state:%s:%s", imei, "batPerc"),
	}
	redisVals := store.GetStatesRedis(keys)
	// construir un map normalizado key->int
	state := map[string]int{
		"in1":     redisVals[keys[0]],
		"in2":     redisVals[keys[1]],
		"ign":     redisVals[keys[2]],
		"move":    redisVals[keys[3]],
		"out1":    redisVals[keys[4]],
		"batVolt": redisVals[keys[5]],
		"extvolt": redisVals[keys[6]],
		"ain1":    redisVals[keys[7]],
		"batPerc": redisVals[keys[8]],
		"sleepM":  sleep,
		"vclSpd":  vehicleSpd,
	}

	// 5) Construir TrackingObject con GPS + estados de Redis y emitir por gRPC
	tr := pipeline.BuildTrackingFromStates(
		imei,
		parsed["timestamp"],
		toFloatAny(parsed["latitude"]),
		toFloatAny(parsed["longitude"]),
		toIntAny(parsed["speed"]),
		toIntAny(parsed["angle"]),
		toIntAny(parsed["satellites"]),
		state,
	)
	lg := observability.NewLogger()
	tr.MsgType = msgType
	tr.Model = model
	tr.FWVer = fwVer
	msgs := pipeline.ToGRPC(tr)
	for _, m := range msgs {
		lg.Info("gRPC payload", "imei", tr.IMEI, "payload", m)
	}
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

func toIntAny(x interface{}) int {
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
	case float32:
		return int(v)
	case float64:
		return int(v)
	case string:
		n, _ := strconv.Atoi(v)
		return n
	default:
		return 0
	}
}

func toFloatAny(x interface{}) float64 {
	switch v := x.(type) {
	case float32:
		return float64(v)
	case float64:
		return v
	case int:
		return float64(v)
	case int8:
		return float64(v)
	case int16:
		return float64(v)
	case int32:
		return float64(v)
	case int64:
		return float64(v)
	case uint:
		return float64(v)
	case uint8:
		return float64(v)
	case uint16:
		return float64(v)
	case uint32:
		return float64(v)
	case uint64:
		return float64(v)
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}
