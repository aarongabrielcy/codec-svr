package pipeline

import (
	"encoding/json"
	"time"
)

// ---------------- helpers de coordenadas / fix ----------------

func coordsValid(lat, lon float64) bool {
	if lat == 0 && lon == 0 {
		return false
	}
	if lat < -90 || lat > 90 || lon < -180 || lon > 180 {
		return false
	}
	return true
}

func CalcFix(sats int, lat, lon float64) int {
	if sats > 3 && coordsValid(lat, lon) {
		return 1
	}
	return 0
}

// msg_type: 1 = live, 0 = buffer
func DecideMsgType(isBatch bool, ts time.Time) int {
	if isBatch {
		return 0
	}
	if !ts.IsZero() && time.Since(ts) > 120*time.Second {
		return 0
	}
	return 1
}

// ---------------- construcción de TrackingObject ----------------

func BuildTracking(
	imei string,
	dt time.Time,
	lat, lon float64,
	spd, crs, sats int,
	perm map[string]uint64,
	msgType int,
	model, fw string,
) *TrackingObject {
	return &TrackingObject{
		IMEI:     imei,
		Model:    model,
		FWVer:    fw,
		Datetime: dt.Format(time.RFC3339),
		Lat:      lat,
		Lon:      lon,
		Spd:      spd,
		Crs:      crs,
		Sats:     sats,
		PermIO:   perm, // plano: "239"->1, "1"->0, etc. (desde Redis)
		MsgType:  msgType,
		Fix:      CalcFix(sats, lat, lon),
	}
}

// ---------------- agrupación perm_io y salida gRPC ---------------

// agrupa el map plano ("239"->1, "66"->12450, ...) en:
//
//	"perm_io": {
//	  "n1": { "1":1, "9":43, ... },
//	  "n2": { "24":30, "29":90, ... },
//	  "n4": { "11":895202092, "14":4280923948, ... },
//	  "n8": { "25":32767, ... }
//	}
//
// NOTA: aquí inferimos el tamaño por el rango numérico del valor:
//
//	<=0xFF -> n1, <=0xFFFF -> n2, <=0xFFFFFFFF -> n4, > eso -> n8.
func groupPermIO(perm map[string]uint64) map[string]map[string]uint64 {
	out := map[string]map[string]uint64{
		"n1": {},
		"n2": {},
		"n4": {},
		"n8": {},
	}

	for id, val := range perm {
		switch {
		case val <= 0xFF:
			out["n1"][id] = val
		case val <= 0xFFFF:
			out["n2"][id] = val
		case val <= 0xFFFFFFFF:
			out["n4"][id] = val
		default:
			out["n8"][id] = val
		}
	}

	// si algún grupo quedó vacío, lo dejamos como nil para que
	// json.Marshal lo omita si usas ,omitempty más adelante
	if len(out["n1"]) == 0 {
		delete(out, "n1")
	}
	if len(out["n2"]) == 0 {
		delete(out, "n2")
	}
	if len(out["n4"]) == 0 {
		delete(out, "n4")
	}
	if len(out["n8"]) == 0 {
		delete(out, "n8")
	}

	return out
}

// ToGRPC ahora arma un JSON real usando encoding/json y
// la agrupación anterior.
func ToGRPC(tr *TrackingObject) []string {
	type payload struct {
		IMEI    string                       `json:"imei"`
		DT      string                       `json:"dt"`
		Lat     float64                      `json:"lat"`
		Lon     float64                      `json:"lon"`
		Spd     int                          `json:"spd"`
		Crs     int                          `json:"crs"`
		Sats    int                          `json:"sats"`
		PermIO  map[string]map[string]uint64 `json:"perm_io"`
		MsgType int                          `json:"msg_type"`
		Fix     int                          `json:"fix"`
		Model   string                       `json:"model,omitempty"`
		FWVer   string                       `json:"fw_ver,omitempty"`
	}

	pl := payload{
		IMEI:    tr.IMEI,
		DT:      tr.Datetime,
		Lat:     tr.Lat,
		Lon:     tr.Lon,
		Spd:     tr.Spd,
		Crs:     tr.Crs,
		Sats:    tr.Sats,
		PermIO:  groupPermIO(tr.PermIO),
		MsgType: tr.MsgType,
		Fix:     tr.Fix,
		Model:   tr.Model,
		FWVer:   tr.FWVer,
	}

	b, err := json.Marshal(pl)
	if err != nil {
		// en caso extremo, devolvemos algo mínimo para no romper el flujo
		return []string{`{"error":"json_marshal_failed"}`}
	}
	return []string{string(b)}
}
