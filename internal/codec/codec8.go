package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// ParseCodec8E parses a full TCP frame that contains a single Codec 8 Extended (0x8E) AVL block.
// It expects the packet starting at preamble (4x00), then data length, then codec, etc.
func ParseCodec8E(data []byte) (map[string]interface{}, error) {
	var off int
	// Basic safety
	if len(data) < 4+4+1+1+8+1 {
		return nil, fmt.Errorf("packet too short: %d", len(data))
	}
	// Skip preamble
	off += 4
	// Data length
	dataLen := int(binary.BigEndian.Uint32(data[off : off+4]))
	off += 4
	if off+dataLen+4 > len(data) {
		return nil, fmt.Errorf("declared data length %d exceeds buffer (off=%d, len=%d)", dataLen, off, len(data))
	}
	codec := data[off]
	off++
	if codec != 0x8E {
		return nil, fmt.Errorf("unexpected codec 0x%X (expected 0x8E)", codec)
	}
	n1 := int(data[off])
	off++
	if n1 <= 0 {
		return nil, fmt.Errorf("no AVL records (n1=%d)", n1)
	}

	// Only one record is typical; parse the first
	// Timestamp (8)
	tsms := binary.BigEndian.Uint64(data[off : off+8])
	off += 8
	priority := data[off]
	off++

	// GPS 15 bytes
	lon := int32(binary.BigEndian.Uint32(data[off : off+4]))
	off += 4
	lat := int32(binary.BigEndian.Uint32(data[off : off+4]))
	off += 4
	altitude := binary.BigEndian.Uint16(data[off : off+2])
	off += 2
	angle := binary.BigEndian.Uint16(data[off : off+2])
	off += 2
	sats := data[off]
	off++
	speed := binary.BigEndian.Uint16(data[off : off+2])
	off += 2

	// IO header (all 2-byte in 8E)
	eventID := binary.BigEndian.Uint16(data[off : off+2])
	off += 2
	totalIO := int(binary.BigEndian.Uint16(data[off : off+2]))
	off += 2

	// Groups
	readU16 := func() (uint16, error) {
		if off+2 > len(data) {
			return 0, fmt.Errorf("buffer overflow on u16 at off=%d", off)
		}
		v := binary.BigEndian.Uint16(data[off : off+2])
		off += 2
		return v, nil
	}
	readU32 := func() (uint32, error) {
		if off+4 > len(data) {
			return 0, fmt.Errorf("buffer overflow on u32 at off=%d", off)
		}
		v := binary.BigEndian.Uint32(data[off : off+4])
		off += 4
		return v, nil
	}
	readU64 := func() (uint64, error) {
		if off+8 > len(data) {
			return 0, fmt.Errorf("buffer overflow on u64 at off=%d", off)
		}
		v := binary.BigEndian.Uint64(data[off : off+8])
		off += 8
		return v, nil
	}
	readU8 := func() (uint8, error) {
		if off+1 > len(data) {
			return 0, fmt.Errorf("buffer overflow on u8 at off=%d", off)
		}
		v := data[off]
		off++
		return v, nil
	}

	type item struct {
		size int
		val  uint64
	}
	ioValues := map[uint16]item{}

	// 1-byte group (count is 2 bytes, each id is 2 bytes, values are 1 byte)
	n1b, err := readU16()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n1b); i++ {
		id, err := readU16()
		if err != nil {
			return nil, err
		}
		v, err := readU8()
		if err != nil {
			return nil, err
		}
		ioValues[id] = item{1, uint64(v)}
	}

	// 2-byte group
	n2b, err := readU16()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n2b); i++ {
		id, err := readU16()
		if err != nil {
			return nil, err
		}
		v, err := readU16()
		if err != nil {
			return nil, err
		}
		ioValues[id] = item{2, uint64(v)}
	}

	// 4-byte group
	n4b, err := readU16()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n4b); i++ {
		id, err := readU16()
		if err != nil {
			return nil, err
		}
		v, err := readU32()
		if err != nil {
			return nil, err
		}
		ioValues[id] = item{4, uint64(v)}
	}

	// 8-byte group
	n8b, err := readU16()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(n8b); i++ {
		id, err := readU16()
		if err != nil {
			return nil, err
		}
		v, err := readU64()
		if err != nil {
			return nil, err
		}
		ioValues[id] = item{8, uint64(v)}
	}

	// X-bytes group
	nxb, err := readU16()
	if err != nil {
		return nil, err
	}
	for i := 0; i < int(nxb); i++ {
		id, err := readU16()
		if err != nil {
			return nil, err
		}
		length, err := readU16()
		if err != nil {
			return nil, err
		}
		if off+int(length) > len(data) {
			return nil, fmt.Errorf("buffer overflow on X-bytes payload id=%d", id)
		}
		// We do not interpret variable-length payloads here; skip
		off += int(length)
		ioValues[id] = item{int(length), 0}
	}

	// Number of Data 2 and CRC
	n2 := int(data[off])
	off++
	if n2 != n1 {
		return nil, fmt.Errorf("n2 (%d) != n1 (%d)", n2, n1)
	}
	// 4 bytes CRC (the last 2 are IBM CRC)
	if off+4 > len(data) {
		return nil, fmt.Errorf("missing CRC")
	}
	crc := hex.EncodeToString(data[off : off+4])
	off += 4

	// Build result
	result := map[string]interface{}{
		"codec_id":     int(codec),
		"records":      n1,
		"timestamp_ms": int64(tsms),
		"timestamp":    time.UnixMilli(int64(tsms)).UTC().Format(time.RFC3339),
		"priority":     int(priority),
		"longitude":    float64(lon) / 1e7,
		"latitude":     float64(lat) / 1e7,
		"altitude":     int(altitude),
		"angle":        int(angle),
		"satellites":   int(sats),
		"speed":        int(speed),
		"event_io_id":  int(eventID),
		"io_total":     totalIO,
		"crc":          crc,
	}

	// Flatten some known IDs for FMC125
	// GSM Signal = ID 21 (1B), External Voltage = ID 66 (2B)
	if v, ok := ioValues[21]; ok {
		result["gsm_signal"] = v.val
	}
	if v, ok := ioValues[66]; ok {
		result["external_voltage_mv"] = v.val
	}
	if v, ok := ioValues[239]; ok {
		result["io_239"] = v.val
	}
	if v, ok := ioValues[240]; ok {
		result["io_240"] = v.val
	}

	// Include raw IO map
	pretty := map[string]map[string]uint64{}
	for id, it := range ioValues {
		key := fmt.Sprintf("%d", id)
		pretty[key] = map[string]uint64{
			"size": uint64(it.size),
			"val":  it.val,
		}
	}
	result["io"] = pretty

	fmt.Printf("[DEBUG] GPS parsed: lat=%.6f, lon=%.6f, alt=%d, sats=%d\n",
		result["latitude"], result["longitude"], result["altitude"], result["satellites"])

	fmt.Printf("[DEBUG] IO 1B count=%d, 2B count=%d, 4B count=%d, 8B count=%d, XB count=%d\n",
		n1b, n2b, n4b, n8b, nxb)

	fmt.Printf("\033[32m[INFO]\033[0m Parsed data OK → ts=%s prio=%d ExtVolt(mV)=%v GSM=%v\n",
		result["timestamp"], result["priority"], result["external_voltage_mv"], result["gsm_signal"])

	return result, nil
}
