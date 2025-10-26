package utilities

import (
	"log"
	"os"
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
