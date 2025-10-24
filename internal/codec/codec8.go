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
	if codecID != 0x8E && codecID != 0x08 {
		return nil, fmt.Errorf("unsupported codec ID: 0x%X", codecID)
	}

	// Número de registros
	numberOfData := int(data[9])

	result["data_field_length"] = dataFieldLength
	result["codec_id"] = fmt.Sprintf("0x%X", codecID)
	result["number_of_data"] = numberOfData

	// Empezamos a leer el primer registro
	offset := 10
	if len(data) < offset+15 {
		return result, fmt.Errorf("data too short for first record")
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

	// Resto de los bytes sin procesar (IO elements, CRC, etc.)
	result["raw_remaining"] = hex.EncodeToString(data[offset:])

	fmt.Printf("Parsed data: %+v\n", result)
	return result, nil
}
