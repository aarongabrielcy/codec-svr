package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// safeRead evita panic si el offset excede el buffer.
func safeRead(data []byte, offset, length int) ([]byte, error) {
	if offset+length > len(data) {
		return nil, fmt.Errorf("buffer overflow: tried to read %d bytes at offset %d (len=%d)", length, offset, len(data))
	}
	return data[offset : offset+length], nil
}

// normalizeIO intenta devolver el valor "útil" dado el id, tamaño y bytes.
// - Devuelve (normalizedInt, rawInt, rawHex)
func normalizeIO(id int, size int, valBytes []byte) (int, int, string) {
	rawHex := hex.EncodeToString(valBytes)
	switch size {
	case 1:
		raw := int(valBytes[0])
		return raw, raw, rawHex
	case 2:
		raw := int(binary.BigEndian.Uint16(valBytes))
		// heurística: muchos dispositivos (FMC125) ponen el valor en el byte alto,
		// resultando en 0x0100 -> 256. Si el byte bajo es 0 y el byte alto es pequeño
		// (<= 0xFF) entonces devolvemos el byte alto.
		if raw&0x00FF == 0 && (raw>>8) <= 0xFF && (raw>>8) != 0 {
			return raw >> 8, raw, rawHex
		}
		// otra heurística: si el raw es muy grande y no parece voltaje (ej > 50000),
		// devolvemos raw >> 8 cuando eso tenga sentido (conservador).
		return raw, raw, rawHex
	case 4:
		raw := int(binary.BigEndian.Uint32(valBytes))
		return raw, raw, rawHex
	case 8:
		raw64 := int(binary.BigEndian.Uint64(valBytes))
		return raw64, raw64, rawHex
	default:
		// fallback - hex representation as raw
		return 0, 0, rawHex
	}
}

func ParseCodec8E(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	if len(data) < 15 {
		return nil, fmt.Errorf("packet too short: %d", len(data))
	}

	// preámbulo
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

	ts, err := safeRead(data, offset, 8)
	if err != nil {
		return result, err
	}
	timestamp := binary.BigEndian.Uint64(ts)
	offset += 8

	priority := data[offset]
	offset++

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

	result["timestamp_ms"] = timestamp
	result["priority"] = priority
	result["longitude"] = float64(longitude) / 10000000
	result["latitude"] = float64(latitude) / 10000000
	result["altitude"] = altitude
	result["angle"] = angle
	result["satellites"] = satellites
	result["speed_kph"] = speed

	// IO header
	if len(data) <= offset+2 {
		return result, fmt.Errorf("data too short for IO header (offset=%d)", offset)
	}
	eventIO := data[offset]
	totalIO := int(data[offset+1])
	offset += 2

	result["event_io_id"] = eventIO
	result["io_total"] = totalIO

	ioElements := make(map[int]map[string]interface{})
	totalRead := 0

	// readGroup lee un grupo de IOs de 'size' bytes (1,2,4,8)
	readGroup := func(size int) error {
		if totalRead >= totalIO {
			return nil
		}
		if offset >= len(data) {
			return fmt.Errorf("unexpected EOF at offset %d", offset)
		}
		count := int(data[offset])
		offset++

		// protección
		if count > 200 {
			count = 0
		}

		// limitar por lo que falta según totalIO
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

			normVal, rawVal, rawHex := normalizeIO(id, size, valBytes)

			ioElements[id] = map[string]interface{}{
				"size":       size,
				"val":        normVal, // valor normalizado para uso
				"raw_val":    rawVal, // valor crudo interpretado
				"raw_hex":    rawHex, // hex de bytes
				"bytes_count": size,
			}
			totalRead++
			if totalRead >= totalIO {
				return nil
			}
		}
		return nil
	}

	// leer 1B,2B,4B,8B
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

	// grupo X (id + size + value) si quedan
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

			normVal, rawVal, rawHex := normalizeIO(id, size, valBytes)

			ioElements[id] = map[string]interface{}{
				"size":       size,
				"val":        normVal,
				"raw_val":    rawVal,
				"raw_hex":    rawHex,
				"bytes_count": size,
			}
			totalRead++
			if totalRead >= totalIO {
				break
			}
		}
	}

	result["io"] = ioElements

	// mapea algunos IOs importantes (ejemplo)
	if v, ok := ioElements[239]; ok {
		result["ignition"] = v["val"]
	}
	if v, ok := ioElements[66]; ok {
		result["external_voltage_mv"] = v["val"]
	}
	if v, ok := ioElements[113]; ok {
		result["battery_level"] = v["val"]
	}
	// también exponer entradas/salidas conocidas
	if v, ok := ioElements[1]; ok {
		result["digital_input_1"] = v["val"]
	}
	if v, ok := ioElements[2]; ok {
		result["digital_input_2"] = v["val"]
	}
	if v, ok := ioElements[179]; ok {
		result["digital_output_1"] = v["val"]
	}

	// CRC (últimos 4 bytes)
	if len(data) >= 4 {
		crcStart := len(data) - 4
		crc := binary.BigEndian.Uint32(data[crcStart:])
		result["crc"] = fmt.Sprintf("%08x", crc)
	}
	// raw remaining (entre offset y CRC)
	if offset < len(data)-4 {
		result["raw_remaining"] = hex.EncodeToString(data[offset : len(data)-4])
	} else {
		result["raw_remaining"] = ""
	}

	// log informativo
	t := time.UnixMilli(int64(timestamp))
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → ts=%s prio=%d\n",
		t.UTC().Format(time.RFC3339), priority)

	return result, nil
}
