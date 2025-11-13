package codec

import (
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"time"
)

// OJO: Usamos IOItem exportado desde avl_models.go (mismo package "codec")
// type IOItem struct {
// 	Size int
// 	Val  uint64
// 	Raw  []byte
// }

func ParseCodec8E(frame []byte) (map[string]interface{}, error) {
	var off int
	if len(frame) < 12 {
		return nil, fmt.Errorf("frame too short")
	}
	// --- preámbulo + dataLen ---
	off += 4
	dataLen := int(binary.BigEndian.Uint32(frame[off : off+4]))
	off += 4
	if off+dataLen+4 > len(frame) {
		return nil, fmt.Errorf("declared len exceeds buffer")
	}

	// --- payload ---
	codec := frame[off]
	off++
	if codec != 0x8E {
		return nil, fmt.Errorf("codec 0x%X != 0x8E", codec)
	}
	n1 := int(frame[off]) // Number of Data 1 (records)
	off++
	if n1 <= 0 {
		return nil, fmt.Errorf("no records")
	}

	// Helpers con control de límites
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

	// Variables del ÚLTIMO record (el más reciente) para devolver en el map
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
		ioVals  map[uint16]IOItem
	)

	// --- Recorrer TODOS los records ---
	for r := 0; r < n1; r++ {
		// Timestamp (8B, ms since epoch)
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

		// Grupos de IO (EN 8E: los CONTADORES son uint16)
		ioThis := map[uint16]IOItem{}

		// 1-byte values
		cnt1, err := readU16()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt1); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v8, err := readU8()
			if err != nil {
				return nil, err
			}
			ioThis[id] = IOItem{Size: 1, Val: uint64(v8)}
		}

		// 2-byte values
		cnt2, err := readU16()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt2); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v16, err := readU16()
			if err != nil {
				return nil, err
			}
			ioThis[id] = IOItem{Size: 2, Val: uint64(v16)}
		}

		// 4-byte values
		cnt4, err := readU16()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt4); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v32, err := readU32()
			if err != nil {
				return nil, err
			}
			ioThis[id] = IOItem{Size: 4, Val: uint64(v32)}
		}

		// 8-byte values
		cnt8, err := readU16()
		if err != nil {
			return nil, err
		}
		for i := 0; i < int(cnt8); i++ {
			id, err := readU16()
			if err != nil {
				return nil, err
			}
			v64, err := readU64()
			if err != nil {
				return nil, err
			}
			ioThis[id] = IOItem{Size: 8, Val: v64}
		}

		// X-bytes values
		cnx, err := readU16()
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
			// Guardamos el contenido X-bytes en Raw por si luego lo quieres usar
			raw := make([]byte, int(l))
			copy(raw, frame[off:off+int(l)])
			off += int(l)

			ioThis[id] = IOItem{Size: int(l), Raw: raw}
		}

		// Guardamos el ÚLTIMO record (normalmente el más reciente)
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

	// Resultado con el ÚLTIMO record
	result := map[string]interface{}{
		"codec_id":    int(codec),
		"records":     n1,
		"qty1":        n1,
		"qty2":        n2,
		"is_batch":    n1 > 1, // útil para msg_type=buffer
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
		"io":          ioVals, // map[uint16]IOItem (tipado)
	}
	return result, nil
}
