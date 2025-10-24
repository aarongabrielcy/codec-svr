package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
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
		return nil, fmt.Errorf("data too short to be valid Codec8E packet")
	}

	// --- Preamble ---
	if data[0] != 0x00 || data[1] != 0x00 || data[2] != 0x00 || data[3] != 0x00 {
		return nil, fmt.Errorf("invalid preamble (expected 0x00000000)")
	}

	dataFieldLength := binary.BigEndian.Uint32(data[4:8])
	codecID := data[8]
	numberOfData := int(data[9])

	result["data_field_length"] = dataFieldLength
	result["codec_id"] = fmt.Sprintf("0x%X", codecID)
	result["number_of_data"] = numberOfData

	offset := 10
	fmt.Printf("[DEBUG] Start parsing AVL: codec=0x%X, records=%d, total_len=%d\n", codecID, numberOfData, len(data))

	// Timestamp
	ts, err := safeRead(data, offset, 8)
	if err != nil {
		return result, err
	}
	timestamp := binary.BigEndian.Uint64(ts)
	offset += 8

	if len(data) < offset+15 {
		return result, fmt.Errorf("data too short for AVL record header (offset=%d)", offset)
	}

	priority := data[offset]
	offset++

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

	result["timestamp"] = timestamp
	result["priority"] = priority
	result["longitude"] = float64(longitude) / 10000000
	result["latitude"] = float64(latitude) / 10000000
	result["altitude"] = altitude
	result["angle"] = angle
	result["satellites"] = satellites
	result["speed_kph"] = speed

	fmt.Printf("[DEBUG] GPS parsed: lat=%.6f, lon=%.6f, alt=%d, sats=%d\n", result["latitude"], result["longitude"], altitude, satellites)

	if len(data) <= offset+2 {
		return result, fmt.Errorf("data too short for IO header (offset=%d)", offset)
	}

	eventIO := data[offset]
	totalIO := data[offset+1]
	offset += 2

	result["event_io_id"] = eventIO
	result["total_io_count"] = totalIO

	ioValues := make(map[uint8]interface{})

	readGroup := func(size int) error {
		if offset >= len(data) {
			return fmt.Errorf("unexpected EOF at offset %d", offset)
		}
		count := int(data[offset])
		offset++
		fmt.Printf("[DEBUG] IO group %dB: count=%d (offset=%d)\n", size, count, offset)

		for i := 0; i < count; i++ {
			if offset+1+size > len(data) {
				return fmt.Errorf("IO section exceeds buffer at id index %d (offset=%d, len=%d)", i, offset, len(data))
			}
			id := data[offset]
			valBytes := data[offset+1 : offset+1+size]
			offset += 1 + size

			switch size {
			case 1:
				ioValues[id] = valBytes[0]
			case 2:
				ioValues[id] = binary.BigEndian.Uint16(valBytes)
			case 4:
				ioValues[id] = binary.BigEndian.Uint32(valBytes)
			case 8:
				ioValues[id] = binary.BigEndian.Uint64(valBytes)
			}
			fmt.Printf("[DEBUG] IO id=%d size=%d val=%v (offset=%d)\n", id, size, ioValues[id], offset)
		}
		return nil
	}

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

	result["io_elements"] = ioValues

	if v, ok := ioValues[239]; ok {
		result["ignition"] = v
	}
	if v, ok := ioValues[200]; ok {
		result["battery_level"] = v
	}
	if v, ok := ioValues[66]; ok {
		result["external_voltage_mv"] = v
	}

	if offset < len(data) {
		result["raw_remaining"] = hex.EncodeToString(data[offset:])
	}

	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → Lat: %.6f, Lon: %.6f, Ign: %v, Batt: %v, ExtVolt: %v\n",
		result["latitude"], result["longitude"], result["ignition"], result["battery_level"], result["external_voltage_mv"])

	return result, nil
}
