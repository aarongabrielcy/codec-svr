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
	result["codec_id"] = fmt.Sprintf("0x%X", codecID)
	result["records"] = numberOfData

	offset := 10
	// Timestamp
	ts, err := safeRead(data, offset, 8)
	if err != nil {
		return result, err
	}
	timestamp := binary.BigEndian.Uint64(ts)
	offset += 8

	// header minimum check for GPS fields
	if len(data) < offset+15 {
		return result, fmt.Errorf("data too short for AVL record header (offset=%d, len=%d)", offset, len(data))
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

	result["timestamp_ms"] = timestamp
	result["priority"] = priority
	result["longitude"] = float64(longitude) / 10000000
	result["latitude"] = float64(latitude) / 10000000
	result["altitude"] = altitude
	result["angle"] = angle
	result["satellites"] = satellites
	result["speed_kph"] = speed

	// IO header: eventIO + totalIO
	if len(data) <= offset+2 {
		return result, fmt.Errorf("data too short for IO header (offset=%d)", offset)
	}
	eventIO := data[offset]
	totalIO := data[offset+1]
	offset += 2

	result["event_io_id"] = eventIO
	result["io_total"] = totalIO

	// Build ioElements as map[int]map[string]interface{}
	ioElements := make(map[int]map[string]interface{})

	// helper to read group of IOs and store into ioElements
	readGroup := func(size int) error {
		if offset >= len(data) {
			return fmt.Errorf("unexpected EOF at offset %d", offset)
		}
		count := int(data[offset])
		offset++
		// sanity cap (avoid bogus counts)
		if count > 50 {
			// sospechoso -> limitar
			count = 0
		}
		for i := 0; i < count; i++ {
			if offset+1+size > len(data) {
				return fmt.Errorf("IO section exceeds buffer at id index %d (offset=%d, len=%d)", i, offset, len(data))
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

	// Parse groups 1B,2B,4B,8B
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

	result["io"] = ioElements

	// remaining raw (crc etc)
	if offset < len(data) {
		result["raw_remaining"] = hex.EncodeToString(data[offset:])
	} else {
		result["raw_remaining"] = ""
	}

	// Debug print
	t := time.UnixMilli(int64(timestamp))
	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → ts=%s prio=%d ExtVolt(mV)=%v GSM=?\n",
		t.UTC().Format(time.RFC3339), priority, ioElements[66])

	return result, nil
}
