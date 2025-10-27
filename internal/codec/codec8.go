package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

func safeRead(data []byte, offset, length int) ([]byte, error) {
	if offset+length > len(data) {
		return nil, fmt.Errorf("buffer overflow: tried to read %d bytes at offset %d (len=%d)", length, offset, len(data))
	}
	return data[offset : offset+length], nil
}

func ParseCodec8E(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	if len(data) < 15 {
		return nil, fmt.Errorf("packet too short: %d", len(data))
	}

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

	if len(data) <= offset+2 {
		return result, fmt.Errorf("data too short for IO header (offset=%d)", offset)
	}
	eventIO := data[offset]
	totalIO := int(data[offset+1])
	offset += 2
	if totalIO == 0 {
		fmt.Printf("[WARN] totalIO=0 (FMC125 workaround enabled) → trying fallback parser...\n")

		ioElements := make(map[int]map[string]interface{})

		if offset >= len(data) {
			return result, fmt.Errorf("data too short for FMC125 fallback")
		}

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

				ioElements[id] = map[string]interface{}{
					"size": size,
					"val":  val,
				}

				fmt.Printf("[DEBUG] IO %dB → ID=%d VAL=%d\n", size, id, val)
			}
		}

		// grupo extendido X (variable size)
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

				val := int(binary.BigEndian.Uint32(append(make([]byte, 4-len(valBytes)), valBytes...)))
				ioElements[id] = map[string]interface{}{
					"size": size,
					"val":  val,
				}
				fmt.Printf("[DEBUG] IO XB → ID=%d VAL=%d\n", id, val)
			}
		}

		result["event_io_id"] = eventIO
		result["io_total"] = len(ioElements)
		result["io"] = ioElements

	} else {
		// ← fallback al parser normal (si el equipo sí usa totalIO > 0)
		ioElements := make(map[int]map[string]interface{})
		totalRead := 0
		readGroup := func(size int) error {
			if totalRead >= totalIO {
				return nil
			}
			if offset >= len(data) {
				return fmt.Errorf("unexpected EOF at offset %d", offset)
			}
			count := int(data[offset])
			offset++
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

		for _, s := range []int{1, 2, 4, 8} {
			if err := readGroup(s); err != nil {
				return result, err
			}
		}
		result["event_io_id"] = eventIO
		result["io_total"] = totalIO
		result["io"] = ioElements
	}
	ioElements := make(map[int]map[string]interface{})
	// totalRead lleva la cuenta de IOs leídos en total (debe ser <= totalIO)
	totalRead := 0
	// Helper para leer grupos (1B,2B,4B,8B)
	readGroup := func(size int) error {
		if totalRead >= totalIO {
			// ya leímos todos los IOs que declararon
			return nil
		}
		if offset >= len(data) {
			return fmt.Errorf("unexpected EOF at offset %d", offset)
		}

		count := int(data[offset])
		offset++

		// no aceptar counts absurdos (protección)
		if count > 200 {
			count = 0
		}

		// limitar a los elementos que faltan por leer según totalIO
		remainingElements := totalIO - totalRead
		if count > remainingElements {
			// ajustar para no sobrepasar el total declarado
			count = remainingElements
		}

		// además limitar por bytes disponibles
		remainingBytes := len(data) - offset
		maxPossible := remainingBytes / (1 + size)
		if count > maxPossible {
			// reducir si no hay suficientes bytes
			count = maxPossible
		}

		for i := 0; i < count; i++ {
			if offset+1+size > len(data) {
				// no hay suficientes bytes para este elemento → salir sin error
				return nil
			}
			id := int(data[offset])
			valBytes := data[offset+1 : offset+1+size]
			offset += 1 + size

			var val interface{}
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

			ioElements[id] = map[string]interface{}{
				"size": size,
				"val":  val,
			}
			totalRead++
			// si ya alcanzamos totalIO, terminamos el grupo
			if totalRead >= totalIO {
				return nil
			}
		}
		return nil
	}

	// leer los grupos en orden 1,2,4,8
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

	// Codec8 Extended: grupo X (id + size + value)
	if totalRead < totalIO && offset < len(data) {
		// count de elementos XB
		countX := int(data[offset])
		offset++
		// Limitar por los que faltan
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
			// si no hay suficientes bytes para el valor, stop
			if offset+size > len(data) {
				break
			}
			valBytes := data[offset : offset+size]
			offset += size

			var val interface{}
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
				val = hex.EncodeToString(valBytes)
			}
			ioElements[id] = map[string]interface{}{
				"size": size,
				"val":  val,
			}
			totalRead++
			if totalRead >= totalIO {
				break
			}
		}
	}

	result["io"] = ioElements

	if ign, ok := ioElements[239]; ok {
		result["ignition"] = ign["val"]
	}
	if din1, ok := ioElements[1]; ok {
		result["digital_input_1"] = din1["val"]
	}
	if dout1, ok := ioElements[179]; ok {
		result["digital_output_1"] = dout1["val"]
	}

	if len(data) >= 4 {
		crcStart := len(data) - 4
		crc := binary.BigEndian.Uint32(data[crcStart:])
		result["crc"] = fmt.Sprintf("%08x", crc)
	}
	if offset < len(data)-4 {
		result["raw_remaining"] = hex.EncodeToString(data[offset : len(data)-4])
	} else {
		result["raw_remaining"] = ""
	}

	t := time.UnixMilli(int64(timestamp))
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → ts=%s prio=%d Ign=%v Din1=%v Dout1=%v\n",
		t.UTC().Format(time.RFC3339), priority,
		result["ignition"], result["digital_input_1"], result["digital_output_1"])

	return result, nil
}
