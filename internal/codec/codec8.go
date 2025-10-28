package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// IO IDs de interés (FMC125: consulta tabla Teltonika para el mapping exacto)
const (
	IOIgnition = 239 // 1B → {0|1}
	IOMovement = 240 // 1B → {0|1}
	IOExtVolt  = 66  // 2B → mV
	IOBattery  = 67  // 1B → %
	IOAin1     = 9   // 2B → raw (depende de perfil)
	IOIn1      = 1   // 1B → {0|1}
	IOIn2      = 2   // 1B → {0|1}
	IOOut1     = 179 // 2B → {0|1} (según config)
)

type ioItem struct {
	Size int
	Val  uint64
}

func ParseCodec8E(frame []byte) (map[string]interface{}, error) {
	var off int
	if len(frame) < 12 {
		return nil, fmt.Errorf("frame too short")
	}
	off += 4 // preamble
	dataLen := int(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	if off+dataLen+4 > len(frame) {
		return nil, fmt.Errorf("declared len exceeds buffer")
	}

	codec := frame[off]
	off++
	if codec != 0x8E {
		return nil, fmt.Errorf("codec 0x%X != 0x8E", codec)
	}
	n1 := int(frame[off])
	off++

	// Solo 1 registro (común). Para múltiples, loop.
	ts := int64(binary.BigEndian.Uint64(frame[off : off+8]))
	off += 8
	priority := frame[off]
	off++

	lon := int32(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	lat := int32(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	alt := binary.BigEndian.Uint16(frame[off : off+2])
	off += 2
	ang := binary.BigEndian.Uint16(frame[off : off+2])
	off += 2
	sats := frame[off]
	off++
	spd := binary.BigEndian.Uint16(frame[off : off+2])
	off += 2

	eventID := binary.BigEndian.Uint16(frame[off : off+2])
	off += 2
	totalIO := int(binary.BigEndian.Uint16(frame[off : off+2]))
	off += 2

	readU16 := func() (uint16, error) {
		if off+2 > len(frame) {
			return 0, fmt.Errorf("oob u16")
		}
		v := binary.BigEndian.Uint16(frame[off : off+2])
		off += 2
		return v, nil
	}
	readU32 := func() (uint32, error) {
		if off+4 > len(frame) {
			return 0, fmt.Errorf("oob u32")
		}
		v := binary.BigEndian.Uint32(frame[off : off+4])
		off += 4
		return v, nil
	}
	readU64 := func() (uint64, error) {
		if off+8 > len(frame) {
			return 0, fmt.Errorf("oob u64")
		}
		v := binary.BigEndian.Uint64(frame[off : off+8])
		off += 8
		return v, nil
	}
	readU8 := func() (uint8, error) {
		if off+1 > len(frame) {
			return 0, fmt.Errorf("oob u8")
		}
		v := frame[off]
		off++
		return v, nil
	}

	ioVals := map[uint16]ioItem{}

	// 1B group
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
		ioVals[id] = ioItem{1, uint64(v)}
	}

	// 2B group
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
		ioVals[id] = ioItem{2, uint64(v)}
	}

	// 4B group
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
		ioVals[id] = ioItem{4, uint64(v)}
	}

	// 8B group
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
		ioVals[id] = ioItem{8, uint64(v)}
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
		l, err := readU16()
		if err != nil {
			return nil, err
		}
		if off+int(l) > len(frame) {
			return nil, fmt.Errorf("oob x-bytes payload")
		}
		off += int(l)
		ioVals[id] = ioItem{int(l), 0}
	}

	// Number of data 2 + CRC
	n2 := int(frame[off])
	off++
	if n2 != n1 {
		return nil, fmt.Errorf("n2 (%d) != n1 (%d)", n2, n1)
	}
	if off+4 > len(frame) {
		return nil, fmt.Errorf("missing CRC")
	}
	crc := hex.EncodeToString(frame[off : off+4])
	off += 4

	result := map[string]interface{}{
		"codec_id":    int(codec),
		"records":     n1,
		"timestamp":   time.UnixMilli(ts).UTC().Format(time.RFC3339),
		"priority":    int(priority),
		"latitude":    float64(lat) / 1e7,
		"longitude":   float64(lon) / 1e7,
		"altitude":    int(alt),
		"angle":       int(ang),
		"satellites":  int(sats),
		"speed":       int(spd),
		"event_io_id": int(eventID),
		"io_total":    totalIO,
		"crc":         crc,
		"io":          ioVals,
	}

	return result, nil
}
