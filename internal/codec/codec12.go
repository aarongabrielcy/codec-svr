package codec

import (
	"encoding/binary"
	"errors"
)

func crc16IBM(b []byte) uint16 {
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

// BuildCodec12 arma un comando Codec 12 (Type=0x05) con el texto ASCII cmd (p.ej. "getver").
// Frame = 00000000 | dataSize(4B) | payload | crc(4B)
// payload = 0x0C | 0x01 | 0x05 | cmdLen(4B) | cmd | 0x01
func BuildCodec12(cmd string) []byte {
	if len(cmd) == 0 {
		// Teltonika espera un texto; evita mandar vacío.
		cmd = "getver"
	}
	putU32 := func(n uint32) []byte { return []byte{byte(n >> 24), byte(n >> 16), byte(n >> 8), byte(n)} }

	cmdBytes := []byte(cmd)
	payload := append([]byte{0x0C, 0x01, 0x05}, putU32(uint32(len(cmdBytes)))...)
	payload = append(payload, cmdBytes...)
	payload = append(payload, 0x01) // Qty2

	dataSize := uint32(len(payload))
	crc := crc16IBM(payload)

	out := make([]byte, 0, 4+4+len(payload)+4)
	out = append(out, 0, 0, 0, 0)                    // preamble
	out = append(out, putU32(dataSize)...)           // data size
	out = append(out, payload...)                    // payload
	out = append(out, 0, 0, byte(crc>>8), byte(crc)) // CRC16 en 4B (00 00 hi lo)
	return out
}

// ParseCodec12Response parsea una respuesta Codec12 (Type=0x06) y devuelve el texto ASCII.
func ParseCodec12Response(frame []byte) (string, error) {
	if len(frame) < 12 {
		return "", errors.New("frame too short")
	}
	// dataSize
	dataLen := int(binary.BigEndian.Uint32(frame[4:8]))
	end := 8 + dataLen
	if end+4 > len(frame) {
		return "", errors.New("incomplete frame")
	}
	payload := frame[8:end]
	crcGot := binary.BigEndian.Uint32(frame[end : end+4]) // 00 00 hi lo

	// Validaciones de payload
	if len(payload) < 8 || payload[0] != 0x0C {
		return "", errors.New("not codec 0x0C")
	}
	// Type debe ser 0x06 (response)
	if payload[2] != 0x06 {
		return "", errors.New("not a response type")
	}
	respSize := int(binary.BigEndian.Uint32(payload[3:7]))
	if 7+respSize+1 > len(payload) {
		return "", errors.New("bad resp size")
	}
	// Qty2 al final del payload
	qty2 := payload[7+respSize]
	if qty2 != 0x01 {
		return "", errors.New("bad qty2")
	}

	// CRC16/IBM sobre payload; va en 4 bytes: 00 00 hi lo
	crcCalc := uint32(crc16IBM(payload))
	crcCalc = uint32(uint16(crcCalc))      // asegurar 16b
	if crcGot != uint32(uint16(crcCalc)) { // tolerante a los 00 00 altos
		return "", errors.New("crc mismatch")
	}

	text := string(payload[7 : 7+respSize]) // ASCII
	return text, nil
}
