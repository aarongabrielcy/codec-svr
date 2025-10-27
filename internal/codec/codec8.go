package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// safeRead previene panic si el offset excede el tamaño del buffer.
func safeRead(data []byte, offset, length int) ([]byte, error) {
	if offset+length > len(data) {
		return nil, fmt.Errorf("buffer overflow: tried to read %d bytes at offset %d (len=%d)", length, offset, len(data))
	}
	return data[offset : offset+length], nil
}

// ParseCodec8E parsea un paquete Codec8 Extended (teltonika).
// Está diseñado para ser tolerante con equipos FMC125 que a veces envían totalIO==0.
func ParseCodec8E(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	if len(data) < 15 {
		return nil, fmt.Errorf("packet too short: %d", len(data))
	}

	// Preamble
	if data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x00 || data[3] != 0x00 {
		return nil, fmt.Errorf("invalid preamble (expected 0x00000000)")
	}

	dataFieldLength := binary.BigEndian.Uint32(data[4:8])
	codecID := data[8]
	numberOfData := int(data[9])

	result["data_field_length"] = dataFieldLength
	result["codec_id"] = int(codecID)
	result["records"] = numberOfData

	offset := 10

	// Timestamp (8 bytes)
	tsBytes, err := safeRead(data, offset, 8)
	if err != nil {
		return result, err
	}
	timestamp := binary.BigEndian.Uint64(tsBytes)
	offset += 8

	// Priority
	priority, errP := safeRead(data, offset, 1)
	if errP != nil {
		return result, errP
	}
	result["timestamp_ms"] = timestamp
	result["priority"] = int(priority[0])
	// keep numeric priority local too
	pri := int(priority[0])
	offset += 1

	// Verificación mínima para campos GPS
	if len(data) < offset+15 {
		return result, fmt.Errorf("data too short for GPS record header (offset=%d, len=%d)", offset, len(data))
	}

	longitude := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	latitude := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4
	altitude := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	angle := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2
	satellites := data[offset]
	offset++
	speed := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	result["longitude"] = float64(longitude) / 10000000
	result["latitude"] = float64(latitude) / 10000000
	result["altitude"] = altitude
	result["angle"] = angle
	result["satellites"] = satellites
	result["speed_kph"] = speed

	// IO header (eventIO + totalIO)
	if len(data) <= offset+1 {
		return result, fmt.Errorf("data too short for IO header (offset=%d)", offset)
	}
	eventIO := data[offset]
	totalIO := int(data[offset+1])
	offset += 2

	// Resultado preliminar
	result["event_io_id"] = eventIO
	result["io_total"] = totalIO

	// ioElements acumulador
	ioElements := make(map[int]map[string]interface{})

	// ---------------------------------------------------------
	// CASE A: totalIO == 0 -> fallback (FMC125 / variant)
	// ---------------------------------------------------------
	if totalIO == 0 {
		fmt.Printf("[WARN] totalIO=0 (FMC125 workaround enabled) → trying fallback parser...\n")

		// grupos por tamaño en orden: 1B,2B,4B,8B
		groupSizes := []int{1, 2, 4, 8}
		for _, size := range groupSizes {
			if offset >= len(data) {
				break
			}
			count := int(data[offset])
			offset++
			if count == 0 {
				continue
			}
			for i := 0; i < count; i++ {
				if offset+1+size > len(data) {
					// no hay suficientes bytes, salir del loop del grupo
					break
				}
				id := int(data[offset])
				valBytes := data[offset+1 : offset+1+size]
				offset += 1 + size

				var val int
				switch size {
				case 1:
					val = int(valBytes[0])
				case 2:
					val = int(binary.BigEndian.Uint16(valBytes))
				case 4:
					val = int(binary.BigEndian.Uint32(valBytes))
				case 8:
					val = int(binary.BigEndian.Uint64(valBytes))
				}
				ioElements[id] = map[string]interface{}{"size": size, "val": val}
				fmt.Printf("[DEBUG] IO %dB → ID=%d VAL=%d\n", size, id, val)
			}
		}

		// grupo extendido X (id + size + value)
		if offset < len(data) {
			countX := int(data[offset])
			offset++
			for i := 0; i < countX; i++ {
				if offset+2 > len(data) {
					break
				}
				id := int(data[offset])
				size := int(data[offset+1])
				offset += 2
				if offset+size > len(data) {
					break
				}
				valBytes := data[offset : offset+size]
				offset += size

				// convertir valBytes (tamaño variable) a int seguro
				val := 0
				switch size {
				case 1:
					val = int(valBytes[0])
				case 2:
					val = int(binary.BigEndian.Uint16(valBytes))
				case 4:
					val = int(binary.BigEndian.Uint32(valBytes))
				case 8:
					val = int(binary.BigEndian.Uint64(valBytes))
				default:
					// si es un tamaño distinto, intenta leerlo como big-endian (hasta 8 bytes)
					tmp := make([]byte, 8)
					copy(tmp[8-len(valBytes):], valBytes)
					val = int(binary.BigEndian.Uint64(tmp))
				}

				ioElements[id] = map[string]interface{}{"size": size, "val": val}
				fmt.Printf("[DEBUG] IO XB → ID=%d VAL=%d\n", id, val)
			}
		}

		// asignar resultados finales (fallback)
		result["io_total"] = len(ioElements)
		result["io"] = ioElements

	} else {
		// ---------------------------------------------------------
		// CASE B: totalIO > 0 -> parser estándar Codec8 Extended
		// ---------------------------------------------------------
		totalRead := 0
		// helper para leer grupos: size = 1,2,4,8
		readGroup := func(size int) error {
			if totalRead >= totalIO {
				return nil
			}
			if offset >= len(data) {
				return fmt.Errorf("unexpected EOF at offset %d", offset)
			}
			count := int(data[offset])
			offset++
			// proteger contra counts absurdos
			if count > 200 {
				count = 0
			}
			// limitar por lo que falta leer según totalIO
			remainingElements := totalIO - totalRead
			if count > remainingElements {
				count = remainingElements
			}
			// limitar por bytes disponibles
			remainingBytes := len(data) - offset
			maxPossible := remainingBytes / (1 + size)
			if count > maxPossible {
				count = maxPossible
			}
			for i := 0; i < count; i++ {
				if offset+1+size > len(data) {
					return nil
				}
				id := int(data[offset])
				valBytes := data[offset+1 : offset+1+size]
				offset += 1 + size

				var val int
				switch size {
				case 1:
					val = int(valBytes[0])
				case 2:
					val = int(binary.BigEndian.Uint16(valBytes))
				case 4:
					val = int(binary.BigEndian.Uint32(valBytes))
				case 8:
					val = int(binary.BigEndian.Uint64(valBytes))
				}
				ioElements[id] = map[string]interface{}{"size": size, "val": val}
				totalRead++
				if totalRead >= totalIO {
					return nil
				}
			}
			return nil
		}

		// leer los grupos en orden
		if err := readGroup(1); err != nil {
			return result, err
		}
		if err := readGroup(2); err != nil {
			return result, err
		}
		if err := readGroup(4); err != nil {
			return result, err
		}
		if err := readGroup(8); err != nil {
			return result, err
		}

		// grupo X (id + size + value) si aún faltan elementos según totalIO
		if totalRead < totalIO && offset < len(data) {
			countX := int(data[offset])
			offset++
			if countX > (totalIO - totalRead) {
				countX = totalIO - totalRead
			}
			for i := 0; i < countX; i++ {
				if offset+2 > len(data) {
					break
				}
				id := int(data[offset])
				size := int(data[offset+1])
				offset += 2
				if offset+size > len(data) {
					break
				}
				valBytes := data[offset : offset+size]
				offset += size

				var val int
				switch size {
				case 1:
					val = int(valBytes[0])
				case 2:
					val = int(binary.BigEndian.Uint16(valBytes))
				case 4:
					val = int(binary.BigEndian.Uint32(valBytes))
				case 8:
					val = int(binary.BigEndian.Uint64(valBytes))
				default:
					tmp := make([]byte, 8)
					copy(tmp[8-len(valBytes):], valBytes)
					val = int(binary.BigEndian.Uint64(tmp))
				}
				ioElements[id] = map[string]interface{}{"size": size, "val": val}
				totalRead++
				if totalRead >= totalIO {
					break
				}
			}
		}

		// asignar resultados finales (parser estándar)
		result["io_total"] = totalRead
		result["io"] = ioElements
	}

	// --- extras: mapear campos útiles ---
	if ign, ok := ioElements[239]; ok {
		result["ignition"] = ign["val"]
	}
	if din1, ok := ioElements[1]; ok {
		result["digital_input_1"] = din1["val"]
	}
	if dout1, ok := ioElements[179]; ok {
		result["digital_output_1"] = dout1["val"]
	}
	// ejemplo de voltajes comunes: id 66 (ext battery mv), 113 (battery %/value)
	if v66, ok := ioElements[66]; ok {
		result["external_voltage_mv"] = v66["val"]
	}
	if v113, ok := ioElements[113]; ok {
		result["battery_value"] = v113["val"]
	}

	// CRC (últimos 4 bytes si están presentes)
	if len(data) >= 4 {
		crcStart := len(data) - 4
		crc := binary.BigEndian.Uint32(data[crcStart:])
		result["crc"] = fmt.Sprintf("%08x", crc)
	}

	// raw_remaining (hasta CRC)
	if offset < len(data)-4 {
		result["raw_remaining"] = hex.EncodeToString(data[offset : len(data)-4])
	} else {
		result["raw_remaining"] = ""
	}

	// Debug: mostrar resumen y contenido final de IOs
	t := time.UnixMilli(int64(timestamp))
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → ts=%s prio=%d io_count=%d\n",
		t.UTC().Format(time.RFC3339), pri, len(ioElements))
	if len(ioElements) > 0 {
		fmt.Printf("[DEBUG] IO ELEMENTS FINAL (%d):\n", len(ioElements))
		for id, v := range ioElements {
			fmt.Printf("   → ID=%d  VAL=%v (size=%v)\n", id, v["val"], v["size"])
		}
	}

	return result, nil
}
