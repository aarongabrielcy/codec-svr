package pipeline

import (
	"fmt"
	"time"
)

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

func DecideMsgType(isBatch bool, ts time.Time) int {
	if isBatch {
		return 0
	}
	if !ts.IsZero() && time.Since(ts) > 120*time.Second {
		return 0
	}
	return 1
}

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
		PermIO:   perm,
		MsgType:  msgType,
		Fix:      CalcFix(sats, lat, lon),
	}
}

func ToGRPC(tr *TrackingObject) []string {
	out := fmt.Sprintf(
		`{"imei":"%s","dt":"%s","lat":%.6f,"lon":%.6f,"spd":%d,"crs":%d,"sats":%d,"perm_io":%v,"msg_type":%d,"fix":%d,"model":"%s","fw_ver":"%s"}`,
		tr.IMEI, tr.Datetime, tr.Lat, tr.Lon, tr.Spd, tr.Crs, tr.Sats,
		tr.PermIO, tr.MsgType, tr.Fix, tr.Model, tr.FWVer,
	)
	return []string{out}
}

// Ejemplo de uso en dispatcher.ProcessIncoming:
// tr, ioRaw, err := pipeline.ParseAVL(imei, frame)
// pipeline.DecodeIO(imei, ioRaw, tr)
// msgs := pipeline.ToGRPC(tr)  // envíalo por tu cliente gRPC
