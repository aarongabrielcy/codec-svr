package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"
	"strings"
	"time"

	"codec-svr/internal/codec"
	"codec-svr/internal/dispatcher"
	"codec-svr/internal/observability"
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
}

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
			}
			continue
		}

		// ---- Extraer frames completos ----
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

			// ---------- RESPUESTA CODEC 12 (comandos)
			if codecID == 0x0C {
				if text, err := codec.ParseCodec12Response(pkt); err == nil {
					dispatcher.HandleGetVerResponse(st.imei, text)
					dispatcher.HandleICCIDResponse(st.imei, text)
				}
				continue
			}

			// ---------- AVL Data
			if codecID == 0x08 || codecID == 0x8E {
				qty1 := int(pkt[9])

				go dispatcher.ProcessIncoming(st.imei, pkt)

				var ack [4]byte
				binary.BigEndian.PutUint32(ack[:], uint32(qty1))
				conn.Write(ack[:])
				observability.RecordsAck.Inc()
				firstAVLACK = true

				// ---- Enviar getver una sola vez ----
				if st.ready && firstAVLACK && !st.sentGetVer {
					cmd := codec.BuildCodec12("getver")
					conn.Write(cmd)
					st.sentGetVer = true
					lg.Info("sent getver", "imei", st.imei)
					continue
				}

				// ============ LÓGICA ICCID ==============
				if st.sentGetVer && !st.sentICCID && !st.sentICCIDFallback {
					model := dispatcher.GetCachedModel(st.imei)

					// Caso 1: modelo vacío → intentar getimeiccid
					if model == "" {
						cmd := codec.BuildCodec12("getimeiccid")
						conn.Write(cmd)
						st.sentICCID = true
						lg.Info("sent ICCID (unknown model)", "imei", st.imei)
						continue
					}

					ml := strings.ToLower(model)

					// Caso 2: familia 650 → fallback directo
					if strings.Contains(ml, "650") {
						cmd := codec.BuildCodec12("getparam 219,220,221")
						conn.Write(cmd)
						st.sentICCIDFallback = true
						lg.Info("sent ICCID fallback (650)", "imei", st.imei)
						continue
					}

					// Caso 3: modelos normales → intentar getimeiccid
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
