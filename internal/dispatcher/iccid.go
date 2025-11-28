package dispatcher

import (
	"codec-svr/internal/store"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

// ---------- Helpers de decodificación ICCID desde uint64 ----------

// Cada chunk es un uint64 que, en memoria, son 8 bytes big-endian.
// Esos 8 bytes contienen dígitos ASCII ('0'–'9') o padding.
// Ejemplo: 4051327829469704249 → bytes → "89520209"
func decodeICCIDChunk(u uint64) string {
	var buf [8]byte
	binary.BigEndian.PutUint64(buf[:], u)

	var sb strings.Builder
	for _, b := range buf {
		if b >= '0' && b <= '9' {
			sb.WriteByte(b)
		}
	}
	return sb.String()
}

// Une 3 partes uint64 (219,220,221) en un ICCID ASCII.
func decodeICCIDFromUintParts(p1, p2, p3 uint64) string {
	return decodeICCIDChunk(p1) + decodeICCIDChunk(p2) + decodeICCIDChunk(p3)
}

// ---------- Manejo de respuestas de comandos ----------

func HandleICCIDResponse(imei string, text string) {
	t := strings.TrimSpace(text)
	lt := strings.ToLower(t)

	// --------- GETIMEICCID ----------
	// Ej: "ICCID: 8952020924380762238"
	if strings.Contains(lt, "iccid:") {
		parts := strings.SplitN(lt, "iccid:", 2)
		if len(parts) < 2 {
			return
		}
		val := strings.TrimSpace(parts[1])
		// por si viene en minúsculas pero son dígitos igual
		val = strings.TrimSpace(val)
		if len(val) >= 18 {
			store.SaveStringSafe("dev:"+imei+":iccid", val)
			fmt.Printf("[ICCID] stored via getimeiccid imei=%s iccid=%s\n", imei, val)
		}
		return
	}

	// ------- GETPARAM 219,220,221 --------
	// Respuesta típica: "Param values: 219:4051327829469704249, 220:3617572717105460786, 221:3617296498359795712"
	if strings.Contains(lt, "param values") {
		m := parseICCIDParts(t) // map[id]decimalString
		s219, ok219 := m[219]
		s220, ok220 := m[220]
		s221, ok221 := m[221]
		if !ok219 || !ok220 || !ok221 {
			return
		}

		u219, err1 := strconv.ParseUint(s219, 10, 64)
		u220, err2 := strconv.ParseUint(s220, 10, 64)
		u221, err3 := strconv.ParseUint(s221, 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}

		iccid := decodeICCIDFromUintParts(u219, u220, u221)
		if len(iccid) >= 18 {
			store.SaveStringSafe("dev:"+imei+":iccid", iccid)
			fmt.Printf("[ICCID] stored via getparam imei=%s iccid=%s\n", imei, iccid)
		}
	}
}

// extrae map[ID]valueDecimal de la respuesta de getparam
func parseICCIDParts(s string) map[int]string {
	out := map[int]string{}
	// ejemplo bruto:
	// "Param values: 219:4051..., 220:3617..., 221:3617..."
	// Cortamos después de "Param values:"
	idx := strings.Index(strings.ToLower(s), "param values:")
	if idx >= 0 {
		s = s[idx+len("param values:"):]
	}
	// ahora esperamos "219:..., 220:..., 221:..."
	chunks := strings.Split(s, ",")
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		var id int
		var val string
		n, _ := fmt.Sscanf(c, "%d:%s", &id, &val)
		if n == 2 {
			out[id] = strings.TrimSpace(val)
		}
	}
	return out
}
