package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

func ParseCodec8E(data []byte) (map[string]interface{}, error) {
	result := make(map[string]interface{})

	if len(data) < 15 {
		return nil, fmt.Errorf("data too short to be valid Codec8E packet")
	}

	// Verificar preámbulo
	if data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x00 || data[3] != 0x00 {
		return nil, fmt.Errorf("invalid preamble (expected 0x00000000)")
	}

	dataFieldLength := binary.BigEndian.Uint32(data[4:8])
	codecID := data[8]
	numberOfData := int(data[9])

	result["data_field_length"] = dataFieldLength
	result["codec_id"] = fmt.Sprintf("0x%X", codecID)
	result["number_of_data"] = numberOfData

	offset := 10 // Primer registro

	if len(data) < offset+30 {
		return result, fmt.Errorf("data too short for AVL record")
	}

	// Timestamp
	timestamp := binary.BigEndian.Uint64(data[offset : offset+8])
	offset += 8

	priority := data[offset]
	offset += 1

	longitude := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4

	latitude := int32(binary.BigEndian.Uint32(data[offset : offset+4]))
	offset += 4

	altitude := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	angle := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	satellites := data[offset]
	offset += 1

	speed := binary.BigEndian.Uint16(data[offset : offset+2])
	offset += 2

	result["timestamp"] = timestamp
	result["priority"] = priority
	result["longitude"] = float64(longitude) / 10000000
	result["latitude"] = float64(latitude) / 10000000
	result["altitude"] = altitude
	result["angle"] = angle
	result["satellites"] = satellites
	result["speed_kph"] = speed

	// --- IO Elements ---
	if len(data) < offset+2 {
		return result, fmt.Errorf("data too short for IO elements")
	}

	eventIO := data[offset]
	totalIO := data[offset+1]
	offset += 2

	result["event_io_id"] = eventIO
	result["total_io_count"] = totalIO

	ioValues := make(map[uint8]interface{})

	// Parse 1-byte IOs
	oneByteCount := int(data[offset])
	offset++
	for i := 0; i < oneByteCount; i++ {
		id := data[offset]
		val := data[offset+1]
		ioValues[id] = val
		offset += 2
	}

	// Parse 2-byte IOs
	twoByteCount := int(data[offset])
	offset++
	for i := 0; i < twoByteCount; i++ {
		id := data[offset]
		val := binary.BigEndian.Uint16(data[offset+1 : offset+3])
		ioValues[id] = val
		offset += 3
	}

	// Parse 4-byte IOs
	fourByteCount := int(data[offset])
	offset++
	for i := 0; i < fourByteCount; i++ {
		id := data[offset]
		val := binary.BigEndian.Uint32(data[offset+1 : offset+5])
		ioValues[id] = val
		offset += 5
	}

	// Parse 8-byte IOs
	eightByteCount := int(data[offset])
	offset++
	for i := 0; i < eightByteCount; i++ {
		id := data[offset]
		val := binary.BigEndian.Uint64(data[offset+1 : offset+9])
		ioValues[id] = val
		offset += 9
	}

	result["io_elements"] = ioValues

	// Mapeamos algunos IO importantes
	if v, ok := ioValues[239]; ok {
		result["ignition"] = v
	}
	if v, ok := ioValues[200]; ok {
		result["battery_level"] = v
	}
	if v, ok := ioValues[66]; ok {
		result["external_voltage_mv"] = v
	}

	result["raw_remaining"] = hex.EncodeToString(data[offset:])

	fmt.Printf("\033[32m[INFO]\033[0m Parsed data: %+v\n", result)
	return result, nil
}
