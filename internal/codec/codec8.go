package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
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

	// --- encabezado general ---
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
	n1 := int(frame[off]) // Number of Data 1
	off++

	if n1 <= 0 {
		return nil, fmt.Errorf("no records")
	}

	// Helpers de lectura desde frame con control de límites
	readU8 := func() (uint8, error) {
		if off+1 > len(frame) {
			return 0, fmt.Errorf("oob u8")
		}
		v := frame[off]
		off++
		return v, nil
	}
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

	// Variables para devolver (del último record)
	var (
		ts       int64
		priority uint8
		lon, lat int32
		alt      uint16
		ang      uint16
		sats     uint8
		spd      uint16

		eventID uint16
		totalIO uint16
		ioVals  map[uint16]ioItem
	)

	// --- Recorrer TODOS los records (para no romper el offset) ---
	for r := 0; r < n1; r++ {
		// Timestamp (8B, ms)
		u64, err := readU64()
		if err != nil {
			return nil, err
		}
		ts = int64(u64)

		// Priority (1B)
		p8, err := readU8()
		if err != nil {
			return nil, err
		}
		priority = p8

		// GPS: lon(4), lat(4), alt(2), ang(2), sats(1), spd(2)
		u32, err := readU32()
		if err != nil {
			return nil, err
		}
		lon = int32(u32)

		u32, err = readU32()
		if err != nil {
			return nil, err
		}
		lat = int32(u32)

		u16, err := readU16()
		if err != nil {
			return nil, err
		}
		alt = u16

		u16, err = readU16()
		if err != nil {
			return nil, err
		}
		ang = u16

		p8, err = readU8()
		if err != nil {
			return nil, err
		}
		sats = p8

		u16, err = readU16()
		if err != nil {
			return nil, err
		}
		spd = u16

		// IO header (8E): event_io_id (2B), total_io (2B)
		u16, err = readU16()
		if err != nil {
			return nil, err
		}
		eventID = u16

		u16, err = readU16()
		if err != nil {
			return nil, err
		}
		totalIO = u16

		// Repositorio de IO de este record
		ioThis := map[uint16]ioItem{}

		// Grupos: para 8E, el "count" es 1B; el ID es 2B
		// 1-byte values
		cnt1, err := readU8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt1); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v, err := readU8()
			if err != nil {
				return nil, err
			}
			ioThis[id] = ioItem{Size: 1, Val: uint64(v)}
		}

		// 2-byte values
		cnt2, err := readU8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt2); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v, err := readU16()
			if err != nil {
				return nil, err
			}
			ioThis[id] = ioItem{Size: 2, Val: uint64(v)}
		}

		// 4-byte values
		cnt4, err := readU8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt4); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v, err := readU32()
			if err != nil {
				return nil, err
			}
			ioThis[id] = ioItem{Size: 4, Val: uint64(v)}
		}

		// 8-byte values
		cnt8, err := readU8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt8); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v, err := readU64()
			if err != nil {
				return nil, err
			}
			ioThis[id] = ioItem{Size: 8, Val: v}
		}

		// X-bytes values
		cnx, err := readU8()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnx); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			l, err := readU16() // longitud del payload del ítem
			if err != nil {
				return nil, err
			}
			if off+int(l) > len(frame) {
				return nil, fmt.Errorf("oob x-bytes payload")
			}
			// Omitimos el contenido; si lo necesitas, cópialo.
			off += int(l)
			ioThis[id] = ioItem{Size: int(l), Val: 0}
		}

		// Mantén para devolver el ÚLTIMO record (normalmente el más reciente)
		ioVals = ioThis
	}

	// Number of Data 2
	if off >= len(frame) {
		return nil, fmt.Errorf("missing qty2")
	}
	n2 := int(frame[off])
	off++
	if n2 != n1 {
		return nil, fmt.Errorf("n2 (%d) != n1 (%d)", n2, n1)
	}

	// CRC (4 bytes)
	if off+4 > len(frame) {
		return nil, fmt.Errorf("missing CRC")
	}
	crc := hex.EncodeToString(frame[off : off+4])
	off += 4

	// Construir resultado con el ÚLTIMO record
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
		"io_total":    int(totalIO),
		"crc":         crc,
		"io":          ioVals,
	}

	return result, nil
}
