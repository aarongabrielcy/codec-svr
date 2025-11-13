package dispatcher

import (
	"codec-svr/internal/codec"
	"codec-svr/internal/observability"
	"codec-svr/internal/pipeline"
	"codec-svr/internal/store"
	"codec-svr/internal/utilities"

	"encoding/hex"
	"fmt"
	"reflect"
	"runtime/debug"
	"sort"
	"time"
)

//var previousStates = make(map[string]map[string]int)

// cache de perm IO por dispositivo para actualizar Redis SOLO si cambia
var previousPermIO = make(map[string]map[uint16]uint64)

func ProcessIncoming(imei string, frame []byte) {
	// Detectar batch (Qty1 > 1)
	isBatch := false
	if len(frame) >= 10 && (frame[8] == 0x08 || frame[8] == 0x8E) {
		isBatch = int(frame[9]) > 1
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

	// ---- construir modelos a partir del parsed (1 record, como en tus tramas) ----
	pkt := fromParsedToModels(parsed) // local helper (abajo)
	rec := pkt.Records[0]

	fmt.Printf("[INFO] Parsed AVL OK: codeid=%X ts=%v prio=%v lat=%.6f lon=%.6f alt=%d ang=%d spd=%d sat=%d\n",
		pkt.CodecID,
		rec.Timestamp.Format(time.RFC3339),
		rec.Priority,
		rec.GPS.Latitude, rec.GPS.Longitude,
		rec.GPS.Altitude, rec.GPS.Angle, rec.GPS.Speed, rec.GPS.Satellites,
	)

	// ---- PERM IO: guarda en Redis SÓLO si cambian ----
	if previousPermIO[imei] == nil {
		previousPermIO[imei] = make(map[uint16]uint64)
	}
	for id, it := range rec.IO {
		// Sólo numéricos (1/2/4/8 bytes); si Size corresponde a N bytes (X-bytes) no tiene Val útil
		if it.Size == 1 || it.Size == 2 || it.Size == 4 || it.Size == 8 {
			old := previousPermIO[imei][id]
			if old != it.Val {
				fmt.Printf("[PERMIO] %s id=%d changed %d -> %d\n", imei, id, old, it.Val)
				previousPermIO[imei][id] = it.Val
				store.HSetPermIO(imei, id, it.Val)
			}
		}
	}
	perm := store.HGetAllPermIO(imei) // mapa string→uint64 para el JSON

	// ---- tu debug IO MAP (mantener) ----
	ioMap := extractIOIntMap(parsed["io"]) // reaprovecha tus helpers
	if len(ioMap) == 0 {
		fmt.Println("[WARN] no IO elements found")
	}
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

	// ---- tu bloque de eventos/estados (no se toca) ----
	/*setState := func(key string, val int) {
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
	batt := getValAny(ioMap, fmc125.BatteryVolt)
	battPerc := getValAny(ioMap, fmc125.BattLevel)
	extmv := getValAny(ioMap, fmc125.ExtVolt)
	ain1 := getValAny(ioMap, fmc125.AIn1)
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

	// ---- leer estados normalizados desde Redis (tu fuente de verdad) ----
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
	}*/

	// ---- Construir TrackingObject NUEVO a partir de avl_models ----
	msgType := pipeline.DecideMsgType(isBatch, rec.Timestamp)
	model := store.GetStringSafe("dev:" + imei + ":model")
	fw := store.GetStringSafe("dev:" + imei + ":fw")

	tr := pipeline.BuildTracking(
		imei,
		rec.Timestamp,
		rec.GPS.Latitude, // ojo: lat, lon ya vienen como float64
		rec.GPS.Longitude,
		rec.GPS.Speed,
		rec.GPS.Angle,
		rec.GPS.Satellites,
		perm,
		msgType,
		model, fw,
	)

	// ---- Emitir gRPC (tu formato actual) ----
	lg := observability.NewLogger()
	for _, m := range pipeline.ToGRPC(tr) {
		lg.Info("gRPC payload", "imei", tr.IMEI, "payload", m)
	}
}

// ------------------------- helpers existentes -------------------------

// convertidor flexible de IO (igual que tenías)
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

func getValAny(ioMap map[int]int, ids ...int) int {
	for _, id := range ids {
		if val, exists := ioMap[id]; exists {
			return val
		}
	}
	return 0
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
		var n int
		fmt.Sscanf(v, "%d", &n)
		return n
	default:
		return 0
	}
}

// fromParsedToModels: mini-puente local (no cambia tu ParseCodec8E)
func fromParsedToModels(parsed map[string]interface{}) codec.AvlPacket {
	p := codec.AvlPacket{
		Preamble: 0,
		Len:      0,
		CodecID:  uint8(toInt(parsed["codec_id"])),
		Qty1:     uint8(toInt(parsed["records"])),
		Qty2:     uint8(toInt(parsed["records"])),
		CRC:      0,
	}

	var ts time.Time
	if s, ok := parsed["timestamp"].(string); ok {
		if t, err := time.Parse(time.RFC3339, s); err == nil {
			ts = t
		}
	}

	gps := codec.GPSData{
		Longitude:  utilities.ToFloatAny(parsed["longitude"]),
		Latitude:   utilities.ToFloatAny(parsed["latitude"]),
		Altitude:   toInt(parsed["altitude"]),
		Angle:      toInt(parsed["angle"]),
		Satellites: toInt(parsed["satellites"]),
		Speed:      toInt(parsed["speed"]),
	}

	ioModel := map[uint16]codec.IOItem{}
	if raw, ok := parsed["io"].(map[uint16]codec.IOItem); ok {
		for id, it := range raw {
			ioModel[id] = codec.IOItem{Size: it.Size, Val: it.Val}
		}
	}

	rec := codec.AVLRecord{
		Timestamp: ts,
		Priority:  toInt(parsed["priority"]),
		GPS:       gps,
		EventIOID: toInt(parsed["event_io_id"]),
		TotalIO:   toInt(parsed["io_total"]),
		IO:        ioModel,
	}
	p.Records = []codec.AVLRecord{rec}
	return p
}
