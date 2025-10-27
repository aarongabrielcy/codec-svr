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
	totalIO := data[offset+1]
	offset += 2

	result["event_io_id"] = eventIO
	result["io_total"] = totalIO

	ioElements := make(map[int]map[string]interface{})

	readGroup := func(size int) error {
		if offset >= len(data) {
			return fmt.Errorf("unexpected EOF at offset %d", offset)
		}
		count := int(data[offset])
		offset++

		if count > 50 {
			count = 0
		}

		remaining := len(data) - offset
		expected := count * (1 + size)
		if expected > remaining {

			count = remaining / (1 + size)
			fmt.Printf("[WARN] Truncating IO group: expected=%d available=%d adjustedCount=%d\n", expected, remaining, count)
		}

		for i := 0; i < count; i++ {
			if offset+1+size > len(data) {

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

	if offset < len(data) {
		count := int(data[offset])
		offset++
		for i := 0; i < count; i++ {
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
