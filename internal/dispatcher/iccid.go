package dispatcher

import (
	"codec-svr/internal/store"
	"encoding/binary"
	"fmt"
	"strconv"
	"strings"
)

/* ------------------ Helpers de Decodificación ------------------ */

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

/* ------------------ Manejo de Respuestas ------------------ */
func HandleICCIDResponse(imei, text string) {

	lt := strings.ToLower(text)
	current := store.GetStringSafe("dev:" + imei + ":iccid")

	/* ---------------- PRIMARY (getimeiccid) ---------------- */
	if strings.Contains(lt, "iccid:") {

		parts := strings.SplitN(lt, "iccid:", 2)
		if len(parts) < 2 {
			return
		}
		val := strings.TrimSpace(parts[1])

		if len(val) >= 18 && val != current {
			store.SaveStringSafe("dev:"+imei+":iccid", val)
			fmt.Printf("[ICCID] stored primary imei=%s iccid=%s\n", imei, val)
		}
		return
	}

	/* ---------------- FALLBACK (219/220/221) ---------------- */
	if strings.Contains(lt, "param values") {

		m := parseParts(text)
		s219, ok1 := m[219]
		s220, ok2 := m[220]
		s221, ok3 := m[221]
		if !ok1 || !ok2 || !ok3 {
			return
		}

		u1, _ := strconv.ParseUint(s219, 10, 64)
		u2, _ := strconv.ParseUint(s220, 10, 64)
		u3, _ := strconv.ParseUint(s221, 10, 64)

		newVal := decodeICCID(u1, u2, u3)

		if len(newVal) >= 18 && newVal != current {
			store.SaveStringSafe("dev:"+imei+":iccid", newVal)
			fmt.Printf("[ICCID] stored fallback imei=%s iccid=%s\n", imei, newVal)
		}
	}
}

func parseParts(s string) map[int]string {
	out := map[int]string{}
	s = strings.ToLower(s)
	idx := strings.Index(s, "param values:")
	if idx >= 0 {
		s = s[idx+len("param values:"):]
	}
	chunks := strings.Split(s, ",")
	for _, c := range chunks {
		c = strings.TrimSpace(c)
		var id int
		var val string
		n, _ := fmt.Sscanf(c, "%d:%s", &id, &val)
		if n == 2 {
			out[id] = strings.TrimSpace(val)
		}
	}
	return out
}
