package dispatcher

import (
	"codec-svr/internal/link"
	"codec-svr/internal/store"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

/* ------------------ Helpers de Decodificación ------------------ */

// Cada chunk es un uint64 que, en memoria, son 8 bytes big-endian.
// Esos bytes contienen dígitos ASCII ('0'–'9') o padding.
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

func decodeICCID(p1, p2, p3 uint64) string {
	return decodeICCIDChunk(p1) + decodeICCIDChunk(p2) + decodeICCIDChunk(p3)
}

func digitsOnly(s string) string {
	var sb strings.Builder
	for _, r := range s {
		if r >= '0' && r <= '9' {
			sb.WriteRune(r)
		}
	}
	return sb.String()
}

/* ------------------ Manejo de Respuestas ------------------ */

func HandleICCIDResponse(imei, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}

	current := store.GetStringSafe("dev:" + imei + ":iccid")
	t := strings.TrimSpace(text)
	lt := strings.ToLower(t)

	/* ---------------- PRIMARY (getimeiccid) ----------------
	   Soporta variantes:
	     - "ICCID: 8952..."
	     - "CCID: 8952..."
	     - "IMEI: ..., CCID: 8952..."
	     - "ICCID : 8952..."
	*/
	if strings.Contains(lt, "iccid") || strings.Contains(lt, "ccid") {
		// buscamos primero "iccid", si no "ccid"
		idx := strings.Index(lt, "iccid")
		if idx < 0 {
			idx = strings.Index(lt, "ccid")
		}
		if idx >= 0 {
			// recortamos desde esa posición en el string ORIGINAL
			sub := t[idx:]
			// buscamos el ':' real
			if pos := strings.Index(sub, ":"); pos >= 0 {
				valRaw := strings.TrimSpace(sub[pos+1:])
				// nos quedamos solo con dígitos
				val := digitsOnly(valRaw)

				if len(val) >= 18 && val != current {
					store.SaveStringSafe("dev:"+imei+":iccid", val)
					fmt.Printf("[ICCID] stored primary imei=%s iccid=%s (raw=%q)\n", imei, val, valRaw)

					link.SendDeviceUpdate(link.DeviceInfo{
						IMEI:  imei,
						FWVer: store.GetStringSafe("dev:" + imei + ":fw"),
						Model: store.GetStringSafe("dev:" + imei + ":model"),
						Brand: "TTKA",
						ICCID: val,
					})
				}
			}
		}
		// IMPORTANTE: si venía ICCID en esta respuesta, no seguimos al fallback
		return
	}

	/* ---------------- FALLBACK (getparam 219,220,221) ----------------
	   Ejemplo de respuesta:
	     "Param values: 219:4051..., 220:3617..., 221:3617..."
	*/
	if strings.Contains(lt, "param values") {
		m := parseParts(t)
		s219, ok1 := m[219]
		s220, ok2 := m[220]
		s221, ok3 := m[221]
		if !ok1 || !ok2 || !ok3 {
			return
		}

		u1, err1 := strconv.ParseUint(s219, 10, 64)
		u2, err2 := strconv.ParseUint(s220, 10, 64)
		u3, err3 := strconv.ParseUint(s221, 10, 64)
		if err1 != nil || err2 != nil || err3 != nil {
			return
		}

		newVal := decodeICCID(u1, u2, u3)
		newVal = digitsOnly(newVal)

		if len(newVal) >= 18 && newVal != current {
			store.SaveStringSafe("dev:"+imei+":iccid", newVal)
			fmt.Printf("[ICCID] stored fallback imei=%s iccid=%s\n", imei, newVal)
			link.SendDeviceUpdate(link.DeviceInfo{
				IMEI:  imei,
				FWVer: store.GetStringSafe("dev:" + imei + ":fw"),
				Model: store.GetStringSafe("dev:" + imei + ":model"),
				Brand: "TTKA",
				ICCID: newVal,
			})
		}
	}
}

// extrae map[ID]valueDecimal de la respuesta de getparam
func parseParts(s string) map[int]string {
	out := map[int]string{}
	ls := strings.ToLower(s)

	idx := strings.Index(ls, "param values:")
	if idx >= 0 {
		s = s[idx+len("param values:"):]
	}

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
