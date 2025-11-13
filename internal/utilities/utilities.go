package utilities

import (
	"log"
	"os"
	"reflect"
	"strconv"
	"time"
)

// CreateLog guarda logs con nombre y contenido
func CreateLog(prefix, message string) {
	filename := "logs/" + prefix + "_" + time.Now().Format("20060102") + ".log"

	// Crear carpeta si no existe
	if _, err := os.Stat("logs"); os.IsNotExist(err) {
		os.Mkdir("logs", 0755)
	}

	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Println("Error creando log:", err)
		return
	}
	defer f.Close()

	logLine := time.Now().Format("15:04:05") + " - " + message + "\n"
	if _, err := f.WriteString(logLine); err != nil {
		log.Println("Error escribiendo log:", err)
	}
}

func ToIntAny(x interface{}) int {
	switch v := x.(type) {
	case int:
		return v
	case int8, int16, int32, int64:
		return int(reflect.ValueOf(v).Int())
	case uint, uint8, uint16, uint32, uint64:
		return int(reflect.ValueOf(v).Uint())
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

func ToFloatAny(x interface{}) float64 {
	switch v := x.(type) {
	case float64:
		return v
	case float32:
		return float64(v)
	case int, int8, int16, int32, int64:
		return float64(reflect.ValueOf(v).Int())
	case uint, uint8, uint16, uint32, uint64:
		return float64(reflect.ValueOf(v).Uint())
	case string:
		f, _ := strconv.ParseFloat(v, 64)
		return f
	default:
		return 0
	}
}
