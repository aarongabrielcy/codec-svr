// internal/dispatcher/dispatcher.go
package dispatcher

import (
	"codec-svr/internal/codec"
	"codec-svr/internal/link"
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

// cache de perm IO por dispositivo para actualizar Redis SOLO si cambian
var previousPermIO = make(map[string]map[uint16]uint64)

func ProcessIncoming(imei string, frame []byte) {
	// Detectar si el frame es batch (Qty1 > 1)
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

	// ---- GPS + metadata a partir de parsed ----
	pkt := fromParsedToModels(parsed)
	if len(pkt.Records) == 0 {
		fmt.Println("[WARN] no AVL records in packet")
		return
	}
	rec := pkt.Records[0]

	fmt.Printf("[INFO] Parsed AVL OK: codeid=%X ts=%v prio=%v lat=%.6f lon=%.6f alt=%d ang=%d spd=%d sat=%d\n",
		pkt.CodecID,
		rec.Timestamp.Format(time.RFC3339),
		rec.Priority,
		rec.GPS.Latitude, rec.GPS.Longitude,
		rec.GPS.Altitude, rec.GPS.Angle, rec.GPS.Speed, rec.GPS.Satellites,
	)

	// ---- PERM IO: extraer IOItems del parsed["io"] ----
	ioItems := extractIOItems(parsed["io"]) // map[uint16]codec.IOItem

	// Guardar en Redis SOLO si cambian (como tu patrón actual)
	if previousPermIO[imei] == nil {
		previousPermIO[imei] = make(map[uint16]uint64)
	}
	for id, it := range ioItems {
		// Sólo numéricos 1/2/4/8 bytes (Nx no tiene Val útil)
		if it.Size == 1 || it.Size == 2 || it.Size == 4 || it.Size == 8 {
			old := previousPermIO[imei][id]
			if old != it.Val {
				fmt.Printf("[PERMIO] %s id=%d changed %d -> %d\n", imei, id, old, it.Val)
				previousPermIO[imei][id] = it.Val
				store.HSetPermIO(imei, id, it.Val)
			}
		}
	}

	// ---------- ICCID DESDE IO 219/220/221 (si es que vienen) ----------
	if p219, ok1 := ioItems[219]; ok1 {
		if p220, ok2 := ioItems[220]; ok2 {
			if p221, ok3 := ioItems[221]; ok3 {
				if p219.Val != 0 && p220.Val != 0 && p221.Val != 0 {
					newICCID := decodeICCID(p219.Val, p220.Val, p221.Val)
					newICCID = digitsOnly(newICCID)

					if len(newICCID) >= 18 {
						currentICCID := store.GetStringSafe("dev:" + imei + ":iccid")
						if newICCID != currentICCID {
							store.SaveStringSafe("dev:"+imei+":iccid", newICCID)
							fmt.Printf("[ICCID] stored from AVL IO imei=%s iccid=%s\n", imei, newICCID)
							link.SendDeviceUpdate(link.DeviceInfo{
								IMEI:  imei,
								FWVer: store.GetStringSafe("dev:" + imei + ":fw"),
								Model: store.GetStringSafe("dev:" + imei + ":model"),
								ICCID: newICCID,
							})
						}
					}
				}
			}
		}
	}

	// Leer TODOS los perm IO de Redis (estado más reciente)
	perm := store.HGetAllPermIO(imei) // map[string]uint64

	// ---- Debug IO MAP (mantener tu salida anterior) ----
	ioMap := extractIOIntMap(parsed["io"])
	if len(ioMap) == 0 {
		fmt.Println("[WARN] no IO elements found")
	}
	ids := make([]int, 0, len(ioMap))
	for id := range ioMap {
		ids = append(ids, id)
	}
	sort.Ints(ids)
	/*fmt.Println("─────────────────────────────")
	fmt.Printf("[DEBUG] IO MAP for IMEI %s\n", imei)
	for _, id := range ids {
		fmt.Printf("  • ID=%d → %d\n", id, ioMap[id])
	}
	fmt.Println("─────────────────────────────")*/

	// ---- Construir TrackingObject con los nuevos helpers ----
	msgType := pipeline.DecideMsgType(isBatch, rec.Timestamp)

	tr := pipeline.BuildTracking(
		imei,
		rec.Timestamp,
		rec.GPS.Latitude,
		rec.GPS.Longitude,
		rec.GPS.Speed,
		rec.GPS.Angle,
		rec.GPS.Satellites,
		perm,
		msgType,
	)

	// ---- Emitir gRPC (perm_io agrupado se hace en ToGRPC) ----
	lg := observability.NewLogger()
	for _, m := range pipeline.ToGRPC(tr) {
		lg.Info("gRPC payload", "imei", tr.IMEI, "payload", m)
	}
	link.SendTracking(tr)
}

// ------------------------- helpers -------------------------

// extractIOIntMap: tu helper original para imprimir IOs en texto
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

// fromParsedToModels: puente entre map[string]interface{} y AvlPacket/AVLRecord
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

	rec := codec.AVLRecord{
		Timestamp: ts,
		Priority:  toInt(parsed["priority"]),
		GPS:       gps,
		EventIOID: toInt(parsed["event_io_id"]),
		TotalIO:   toInt(parsed["io_total"]),
		IO:        map[uint16]codec.IOItem{}, // IO real lo manejamos con extractIOItems
	}
	p.Records = []codec.AVLRecord{rec}
	return p
}

// extractIOItems: toma parsed["io"] (map[uint16]X) y lo convierte a map[uint16]codec.IOItem
// respetando Size y Val, sin importar si el tipo subyacente es codec.IOItem o ioItem privado.
func extractIOItems(ioAny interface{}) map[uint16]codec.IOItem {
	out := make(map[uint16]codec.IOItem)
	if ioAny == nil {
		return out
	}

	rv := reflect.ValueOf(ioAny)
	if rv.Kind() != reflect.Map {
		return out
	}

	for _, mk := range rv.MapKeys() {
		// clave -> uint16
		var id uint16
		switch k := mk.Interface().(type) {
		case uint16:
			id = k
		case uint8:
			id = uint16(k)
		case int:
			id = uint16(k)
		case int32:
			id = uint16(k)
		case int64:
			id = uint16(k)
		default:
			continue
		}

		mv := rv.MapIndex(mk)
		if !mv.IsValid() {
			continue
		}
		v := mv.Interface()

		switch t := v.(type) {
		// Si ya es codec.IOItem
		case codec.IOItem:
			out[id] = t

		// Si es tu ioItem privado u otro struct con campos Size y Val
		default:
			sv := reflect.ValueOf(v)
			if sv.Kind() == reflect.Struct {
				fSize := sv.FieldByName("Size")
				fVal := sv.FieldByName("Val")
				var size int
				var val uint64

				if fSize.IsValid() && fSize.CanInterface() {
					size = toInt(fSize.Interface())
				}
				if fVal.IsValid() && fVal.CanInterface() {
					switch vv := fVal.Interface().(type) {
					case uint64:
						val = vv
					default:
						val = uint64(toInt(vv))
					}
				}
				out[id] = codec.IOItem{Size: size, Val: val}
			}
		}
	}
	return out
}
