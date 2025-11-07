package codec

import (
	"encoding/binary"
	"errors"
)

// BuildCodec12 arma un comando Codec 12 (Type=0x05) con el texto ASCII cmd (p.ej. "getver").
// Frame = 00000000 | dataSize(4B) | payload | crc(4B)
// payload = 0x0C | 0x01 | 0x05 | cmdLen(4B) | cmd | 0x01
func BuildCodec12(cmd string) []byte {
	putU32 := func(n uint32) []byte { return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)} }
	crc16IBM := func(b []byte) uint16 {
		var crc uint16
		for _, v := range b {
			crc ^= uint16(v)
			for i := 0; i < 8; i++ {
				if (crc & 1) == 1 {
					crc = (crc >> 1) ^ 0xA001
				} else {
					crc >>= 1
				}
			}
		}
		return crc
	}

	cmdBytes := []byte(cmd)
	payload := append([]byte{0x0C, 0x01, 0x05}, putU32(uint32(len(cmdBytes)))...)
	payload = append(payload, cmdBytes...)
	payload = append(payload, 0x01)

	dataSize := uint32(len(payload))
	crc := crc16IBM(payload)

	out := make([]byte, 0, 4+4+len(payload)+4)
	out = append(out, 0, 0, 0, 0)                    // preamble
	out = append(out, putU32(dataSize)...)           // data size
	out = append(out, payload...)                    // payload
	out = append(out, 0, 0, byte(crc>>8), byte(crc)) // CRC en 4B (alto=0,0)
	return out
}

// ParseCodec12Response parsea una respuesta Codec12 (Type=0x06) y devuelve el texto ASCII.
func ParseCodec12Response(frame []byte) (string, error) {
	if len(frame) < 12 {
		return "", errors.New("frame too short")
	}
	// 0..3 preamble, 4..7 dataSize
	dataLen := int(binary.BigEndian.Uint32(frame[4:8]))
	if 8+dataLen+4 > len(frame) {
		return "", errors.New("incomplete frame")
	}
	payload := frame[8 : 8+dataLen]

	if len(payload) < 1 || payload[0] != 0x0C {
		return "", errors.New("not codec 0x0C")
	}
	if len(payload) < 3 || payload[2] != 0x06 { // Type=0x06 => response
		return "", errors.New("not a response type")
	}
	if len(payload) < 7 {
		return "", errors.New("payload too short")
	}
	respSize := int(binary.BigEndian.Uint32(payload[3:7]))
	if 7+respSize+1 > len(payload) {
		return "", errors.New("bad resp size")
	}
	text := string(payload[7 : 7+respSize]) // ASCII
	// payload[7+respSize] debería ser Qty2=0x01 (lo podrías validar si quieres)
	return text, nil
}
