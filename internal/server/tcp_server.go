package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"time"

	"codec-svr/internal/codec"
	"codec-svr/internal/dispatcher"
	"codec-svr/internal/link"
	"codec-svr/internal/observability"
	"codec-svr/internal/store"
)

type connState struct {
	imei  string
	buf   bytes.Buffer
	ready bool
	log   *slog.Logger

	sentGetVer        bool
	sentICCID         bool
	sentICCIDFallback bool

	sessionOpen time.Time

	// --- reintentos GETVER ---
	getVerAttempts    int
	lastGetVerAttempt time.Time
}

// -------------------------------------------------------------------

func Start(addr string) error {
	lg := observability.NewLogger()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	lg.Info("tcp listening", "addr", addr)

	for {
		conn, err := ln.Accept()
		if err != nil {
			lg.Error("accept", "err", err)
			continue
		}
		observability.TCPConnections.Inc()

		go handleConn(conn, lg.With("remote", conn.RemoteAddr().String()))
	}
}

// -------------------------------------------------------------------

func handleConn(conn net.Conn, lg *slog.Logger) {
	defer conn.Close()
	var st connState
	st.log = lg

	tmp := make([]byte, 4096)
	firstAVLACK := false

	for {
		n, err := conn.Read(tmp)
		if err != nil {
			if err != io.EOF {
				lg.Error("read", "err", err)
			}
			return
		}
		if n == 0 {
			continue
		}
		st.buf.Write(tmp[:n])

		// ---- Handshake IMEI ----
		if st.imei == "" {
			if tryReadIMEI(&st) {
				lg.Info("handshake OK", "imei", st.imei)
				conn.Write([]byte{0x01})
				st.ready = true
				st.sessionOpen = time.Now()

				// -------- device_connect hacia el proxy --------
				host, portStr, _ := net.SplitHostPort(conn.RemoteAddr().String())
				port, _ := strconv.Atoi(portStr)

				info := link.DeviceInfo{
					IMEI:       st.imei,
					FWVer:      store.GetStringSafe("dev:" + st.imei + ":fw"),
					Model:      store.GetStringSafe("dev:" + st.imei + ":model"),
					Brand:      "TTKA",
					ICCID:      store.GetStringSafe("dev:" + st.imei + ":iccid"),
					RemoteIP:   host,
					RemotePort: port,
					State:      link.DeviceStateConnect,
				}
				link.SendDeviceConnect(info)
			}
			continue
		}

		// ---- Procesar frames ----
		for {
			pkt := tryReadAVLFrame(&st.buf)
			if pkt == nil {
				break
			}

			observability.PacketsRecv.Inc()
			if len(pkt) < 13 {
				lg.Warn("short frame", "len", len(pkt))
				continue
			}

			codecID := pkt[8]

			// =====================================================
			//      RESPUESTA CODEC 12 (comandos)
			// =====================================================
			if codecID == 0x0C {

				// DEBUG — ver frame RAW de las respuestas de comando
				//lg.Warn("CODEC12 RAW RESPONSE", "hex", hex.EncodeToString(pkt))

				if text, err := codec.ParseCodec12Response(pkt); err == nil {
					dispatcher.HandleGetVerResponse(st.imei, text)
					dispatcher.HandleICCIDResponse(st.imei, text)
				} else {
					lg.Warn("codec12: frame not parsed", "err", err)
				}
				continue
			}

			// =====================================================
			//          AVL FRAME
			// =====================================================
			if codecID == 0x08 || codecID == 0x8E {
				qty1 := int(pkt[9])

				go dispatcher.ProcessIncoming(st.imei, pkt)

				var ack [4]byte
				binary.BigEndian.PutUint32(ack[:], uint32(qty1))
				conn.Write(ack[:])
				observability.RecordsAck.Inc()
				firstAVLACK = true

				// =====================================================
				//      GETVER con reintentos
				// =====================================================
				if st.ready && firstAVLACK {
					maybeSendGetVer(&st, conn)
				}

				// =====================================================
				//   *** FLUJO ICCID ORIGINAL ***
				// =====================================================
				if st.sentGetVer && !st.sentICCID && !st.sentICCIDFallback {
					model := dispatcher.GetCachedModel(st.imei)

					// Caso 1: modelo desconocido → intentar getimeiccid
					if model == "" {
						cmd := codec.BuildCodec12("getimeiccid")
						conn.Write(cmd)
						st.sentICCID = true
						lg.Info("sent ICCID (unknown model)", "imei", st.imei)
						continue
					}

					ml := strings.ToLower(model)

					// Caso 2: familia 650 -> fallback directo
					if strings.Contains(ml, "650") {
						cmd := codec.BuildCodec12("getparam 219,220,221")
						conn.Write(cmd)
						st.sentICCIDFallback = true
						lg.Info("sent ICCID fallback (650)", "imei", st.imei)
						continue
					}

					// Caso 3: otros modelos -> getimeiccid normal
					cmd := codec.BuildCodec12("getimeiccid")
					conn.Write(cmd)
					st.sentICCID = true
					lg.Info("sent ICCID via getimeiccid", "imei", st.imei)
				}

				continue
			}

			lg.Warn("unknown codec", "id", fmt.Sprintf("0x%02X", codecID))
		}
	}
}

// -------------------------------------------------------------------

func tryReadIMEI(st *connState) bool {
	if st.buf.Len() < 2 {
		return false
	}
	peek := st.buf.Bytes()
	imeiLen := int(binary.BigEndian.Uint16(peek[:2]))
	if imeiLen < 8 || imeiLen > 20 {
		return false
	}
	if st.buf.Len() < 2+imeiLen {
		return false
	}

	st.buf.Next(2)
	imeiBytes := st.buf.Next(imeiLen)

	for _, b := range imeiBytes {
		if b < '0' || b > '9' {
			return false
		}
	}

	st.imei = string(imeiBytes)
	return true
}

func tryReadAVLFrame(buf *bytes.Buffer) []byte {
	if buf.Len() < 12 {
		return nil
	}
	peek := buf.Bytes()
	if !(peek[0] == 0 && peek[1] == 0 && peek[2] == 0 && peek[3] == 0) {
		discard := bytes.Index(peek, []byte{0, 0, 0, 0})
		if discard < 0 {
			buf.Reset()
			return nil
		}
		buf.Next(discard)
		if buf.Len() < 12 {
			return nil
		}
		peek = buf.Bytes()
	}
	dataLen := int(binary.BigEndian.Uint32(peek[4:8]))
	frameLen := 4 + 4 + dataLen + 4
	if buf.Len() < frameLen {
		return nil
	}
	return buf.Next(frameLen)
}

// -------------------------------------------------------------------
//              ** NUEVO: LÓGICA DE REINTENTOS GETVER **
// -------------------------------------------------------------------

func maybeSendGetVer(st *connState, conn net.Conn) {
	const (
		maxSessionAttempts = 3
		minInterval        = 5 * time.Minute
		maxDailyAttempts   = 10
		cmdName            = "getver"
	)

	if st.imei == "" {
		return
	}

	now := time.Now()

	// 1. Si ya lo mandaste una vez, st.sentGetVer se mantiene
	// Pero ahora lo usaremos como "primer intento enviado"
	// y reintentos vendrán por esta función.

	fw := store.GetStringSafe("dev:" + st.imei + ":fw")
	model := store.GetStringSafe("dev:" + st.imei + ":model")

	// 2. Si ya tenemos valores → no reintentar
	if fw != "" && model != "" {
		return
	}

	// 3. Límite por sesión
	if st.getVerAttempts >= maxSessionAttempts {
		st.log.Info("getver session limit reached",
			"imei", st.imei,
			"attempts", st.getVerAttempts)
		return
	}

	// 4. Mínimo tiempo entre intentos
	if !st.lastGetVerAttempt.IsZero() &&
		now.Sub(st.lastGetVerAttempt) < minInterval {
		return
	}

	// 5. Límite diario global por IMEI
	allowed, dailyCount, err := store.IncDailyCmdCounter(st.imei, cmdName, maxDailyAttempts)
	if err != nil {
		st.log.Warn("redis counter failed for getver", "err", err)
		return
	}
	if !allowed {
		st.log.Info("daily getver limit reached",
			"imei", st.imei,
			"count", dailyCount)
		return
	}

	// ---- Enviar GETVER ----
	cmd := codec.BuildCodec12("getver")
	conn.Write(cmd)

	st.sentGetVer = true
	st.getVerAttempts++
	st.lastGetVerAttempt = now

	st.log.Info("sent getver",
		"imei", st.imei,
		"session_attempt", st.getVerAttempts,
		"daily_attempt", dailyCount,
	)
}
