package codec

import (
	"encoding/hex"
	"fmt"
	"log"
	"time"
)

// IO IDs de interés
const (
	IOIgnition    = 239
	IOBattery     = 67
	IOExternalV   = 66
	IOMovement    = 240
	IOInput1      = 1
	IOInput2      = 2
	IOOutput1     = 179
	IOAnalogInput = 9
)

// Get valor del mapa IO (devuelve 0 si no existe)
func getIO(io map[int]int, id int) int {
	if v, ok := io[id]; ok {
		return v
	}
	return 0
}

// Estructura básica del paquete AVL
type AVLData struct {
	Timestamp time.Time
	Priority  byte
	GPS       map[string]interface{}
	IO        map[int]int
}

func ParseCodec8E(imei string, data []byte) (*AVLData, error) {
	if len(data) < 2 {
		return nil, fmt.Errorf("paquete inválido")
	}

	// Extraemos datos básicos simulando decoder previo
	// (no modificamos tu lógica existente)
	avl := &AVLData{
		Timestamp: time.Now(),
		Priority:  data[0],
		GPS: map[string]interface{}{
			"lat": 0,
			"lng": 0,
		},
		IO: make(map[int]int),
	}

	// Ejemplo: llenar con IOs de prueba (esto ya lo hace tu decoder real)
	// Este bloque se reemplaza con el parser binario existente en tu implementación.
	// Aquí solo simulamos datos.
	avl.IO[IOIgnition] = 256
	avl.IO[IOBattery] = 256
	avl.IO[IOExternalV] = 1280
	avl.IO[IOInput1] = 0
	avl.IO[IOInput2] = 0
	avl.IO[IOOutput1] = 512
	avl.IO[IOMovement] = 0
	avl.IO[IOAnalogInput] = 12271

	return avl, nil
}

// Procesa los IOs, interpreta los valores y genera logs de estado/evento
func ProcessIOState(imei string, ioMap map[int]int, prev map[string]int) map[string]int {
	// Leer valores crudos
	ignVal := getIO(ioMap, IOIgnition)
	batVal := getIO(ioMap, IOBattery)
	extVal := getIO(ioMap, IOExternalV)
	in1Val := getIO(ioMap, IOInput1)
	in2Val := getIO(ioMap, IOInput2)
	out1Val := getIO(ioMap, IOOutput1)
	moveVal := getIO(ioMap, IOMovement)
	ain1Val := getIO(ioMap, IOAnalogInput)

	// Interpretar valores
	ignOn := ignVal > 0
	batOn := batVal > 0
	moveOn := moveVal > 0
	in1On := in1Val > 0
	in2On := in2Val > 0
	out1On := out1Val > 0
	extVolt := float64(extVal) / 100.0
	analog1 := float64(ain1Val) / 1000.0 // Teltonika mV → V

	// Log de estado
	log.Printf("[STATE] IMEI=%s Ignition=%v(%d) Battery=%v(%d) ExtVolt=%.2fV(%d) In1=%v(%d) In2=%v(%d) Out1=%v(%d) Move=%v(%d) Analog1=%.3fV(%d)",
		imei,
		ignOn, ignVal,
		batOn, batVal,
		extVolt, extVal,
		in1On, in1Val,
		in2On, in2Val,
		out1On, out1Val,
		moveOn, moveVal,
		analog1, ain1Val,
	)

	// Detección de cambios
	if prev != nil {
		checkChange(imei, "ignition", prev["ign"], ignOn)
		checkChange(imei, "battery", prev["bat"], batOn)
		checkChange(imei, "move", prev["move"], moveOn)
		checkChange(imei, "input1", prev["in1"], in1On)
		checkChange(imei, "input2", prev["in2"], in2On)
		checkChange(imei, "output1", prev["out1"], out1On)
		checkChangeF(imei, "extvolt", float64(prev["ext"]), extVolt)
		checkChangeF(imei, "analog1", float64(prev["ain1"])/1000.0, analog1)
	}

	// Actualizamos estado previo (para próxima lectura)
	state := map[string]int{
		"ign":  boolToInt(ignOn),
		"bat":  boolToInt(batOn),
		"move": boolToInt(moveOn),
		"in1":  boolToInt(in1On),
		"in2":  boolToInt(in2On),
		"out1": boolToInt(out1On),
		"ext":  extVal,
		"ain1": ain1Val,
	}
	return state
}

// Utilidades ---------------------------------------------------

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// Reporta cambio booleano
func checkChange(imei, name string, prev int, curr bool) {
	prevOn := prev > 0
	if prevOn != curr {
		log.Printf("[EVENT] %s %s changed %s → %s",
			imei, name, onOff(prevOn), onOff(curr))
	}
}

// Reporta cambio analógico
func checkChangeF(imei, name string, prev, curr float64) {
	if int(prev*100) != int(curr*100) { // tolerancia mínima
		log.Printf("[EVENT] %s %s changed %.2fV → %.2fV",
			imei, name, prev, curr)
	}
}

func onOff(b bool) string {
	if b {
		return "ON"
	}
	return "OFF"
}

// Helper para debug binario
func DumpHex(label string, b []byte) {
	log.Printf("%s: %s", label, hex.EncodeToString(b))
}
