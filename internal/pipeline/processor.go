package pipeline

import (
	"fmt"
)

type TrackingObject struct {
	IMEI       string            `json:"imei"`
	Datetime   string            `json:"datetime"`
	Latitude   float64           `json:"latitud"`
	Longitude  float64           `json:"longitud"`
	Speed      int               `json:"speed"`
	Course     int               `json:"course"`
	Satellites int               `json:"satellites"`
	Inputs     map[string]int    `json:"inputs"`
	Outputs    map[string]int    `json:"outputs"`
	Extras     map[string]uint64 `json:"extras"`
}

func BuildTrackingFromStates(
	imei string,
	timestamp any,
	lat, lon float64,
	speed, course, sats int,
	state map[string]int,
) *TrackingObject {
	tsStr, _ := timestamp.(string)
	tr := &TrackingObject{
		IMEI:       imei,
		Datetime:   tsStr,
		Latitude:   lat,
		Longitude:  lon,
		Speed:      speed,
		Course:     course,
		Satellites: sats,
		Inputs:     map[string]int{},
		Outputs:    map[string]int{},
		Extras:     map[string]uint64{},
	}
	// Inputs
	tr.Inputs["in1"] = state["in1"]
	tr.Inputs["in2"] = state["in2"]
	tr.Inputs["ign"] = state["ign"]
	tr.Inputs["move"] = state["move"]

	// Outputs
	tr.Outputs["out1"] = state["out1"]

	// Extras (batería %, ext volt mV, ain1 raw)
	tr.Extras["battery_mv"] = uint64(state["batVolt"])
	tr.Extras["ext_voltage_mv"] = uint64(state["extvolt"])
	tr.Extras["ain1_raw"] = uint64(state["ain1"])
	tr.Extras["battery_pct"] = uint64(state["batPerc"])
	tr.Extras["sleep_mode"] = uint64(state["sleepM"])
	tr.Extras["vehicle_speed"] = uint64(state["vclSpd"])
	return tr
}

// ParseAVL: decodifica el paquete, llena datos básicos y retorna io map crudo
/*func ParseAVL(imei string, frame []byte) (*TrackingObject, map[uint16]codecIoItem, error) {
	parsed, err := codec.ParseCodec8E(frame)
	if err != nil {
		return nil, nil, err
	}
	tr := &TrackingObject{
		IMEI:       imei,
		Datetime:   parsed["timestamp"].(string),
		Latitude:   parsed["latitude"].(float64),
		Longitude:  parsed["longitude"].(float64),
		Speed:      parsed["speed"].(int),
		Course:     parsed["angle"].(int),
		Satellites: parsed["satellites"].(int),
		Inputs:     map[string]int{},
		Outputs:    map[string]int{},
		Extras:     map[string]uint64{},
	}
	raw := parsed["io"].(map[uint16]codecIoItem)
	return tr, raw, nil
}*/

// codecIoItem debe igualar el tipo en codec (ioItem)
/*type codecIoItem struct {
	Size int
	Val  uint64
}*/

// DecodeIO: mapea IOs conocidos (FMC125) y guarda estados en Redis
/*func DecodeIO(imei string, raw map[uint16]codecIoItem, tr *TrackingObject) {
	get := func(id uint16) (codecIoItem, bool) {
		v, ok := raw[id]
		return v, ok
	}

	// Inputs/outputs típicos
	if v, ok := get(codec.IOIn1); ok {
		tr.Inputs["in1"] = int(v.Val)
	}
	if v, ok := get(codec.IOIn2); ok {
		tr.Inputs["in2"] = int(v.Val)
	}

	// Ignition / Movement / Outputs
	if v, ok := get(codec.IOIgnition); ok {
		tr.Inputs["ignition"] = int(v.Val)
	} // 0|1
	if v, ok := get(codec.IOMovement); ok {
		tr.Inputs["movement"] = int(v.Val)
	} // 0|1
	if v, ok := get(codec.IOOut1); ok {
		tr.Outputs["out1"] = int(v.Val)
	} // 0|1 (depende config)

	// Battery % (1B) y External Voltage (mV)
	if v, ok := get(codec.IOBattLevel); ok {
		tr.Extras["battery_pct"] = v.Val
	} // 0..100
	if v, ok := get(codec.IOExtVolt); ok {
		tr.Extras["ext_voltage_mv"] = v.Val
	} // mV
	if v, ok := get(codec.IOBatteryVolt); ok {
		tr.Extras["battery_mv"] = v.Val
	} // mV

	// Ejemplo de AIN1
	if v, ok := get(codec.IOAin1); ok {
		tr.Extras["ain1_raw"] = v.Val
	}
	if v, ok := get(codec.IOSleepMode); ok {
		tr.Extras["sleep_mode"] = v.Val
	}

	// Persistimos estados “último valor” para comparar cambios (TTL 10 min en store.SaveEventRedis)
	save := func(key string, val int) {
		store.SaveEventRedisSafe(fmt.Sprintf("state:%s:%s", imei, key), val)
	}

	for k, val := range tr.Inputs {
		save(k, val)
	}
	for k, val := range tr.Outputs {
		save(k, val)
	}
}*/

func ToGRPC(tr *TrackingObject) []string {
	// Aquí conviertes a tu mensaje gRPC real; de momento, devolvemos una sola línea JSON-like
	out := fmt.Sprintf(`{"imei":"%s","dt":"%s","lat":%.6f,"lon":%.6f,"spd":%d,"crs":%d,"sats":%d,"in":%v,"out":%v,"Ext":%v}`,
		tr.IMEI, tr.Datetime, tr.Latitude, tr.Longitude, tr.Speed, tr.Course, tr.Satellites, tr.Inputs, tr.Outputs, tr.Extras)
	return []string{out}
}

// Ejemplo de uso en dispatcher.ProcessIncoming:
// tr, ioRaw, err := pipeline.ParseAVL(imei, frame)
// pipeline.DecodeIO(imei, ioRaw, tr)
// msgs := pipeline.ToGRPC(tr)  // envíalo por tu cliente gRPC
