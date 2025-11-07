package server

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"log/slog"
	"net"

	"codec-svr/internal/codec"
	"codec-svr/internal/dispatcher"
	"codec-svr/internal/observability"
)

type connState struct {
	imei       string
	buf        bytes.Buffer
	ready      bool
	log        *slog.Logger
	sentGetVer bool
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
		// ── (A) Handshake: leer IMEI y ACK 0x01 ─────────────────────────────
		if st.imei == "" {
			if tryReadIMEI(&st) {
				lg.Info("handshake OK", "imei", st.imei)
				if _, err := conn.Write([]byte{0x01}); err != nil {
					lg.Error("handshake ack write failed", "err", err)
					return
				}
				// Marcamos ready tras handshake; enviaremos getver luego del 1er ACK AVL
				st.ready = true
			} else {
				// Aún no hay suficientes bytes para IMEI; leer más del socket
				continue
			}
		}
		// ── (B) Extraer frames: Codec12 primero, luego AVL ──────────────────
		for {
			pkt := tryReadAVLFrame(&st.buf)
			if pkt == nil {
				break
			}
			observability.PacketsRecv.Inc()
			// Seguridad básica por tamaño
			if len(pkt) < 13 {
				lg.Warn("short frame", "len", len(pkt))
				continue
			}
			codecID := pkt[8]
			// (B1) Respuesta de comando (Codec 0x0C) => NO ACK
			if codecID == 0x0C {
				if text, err := codec.ParseCodec12Response(pkt); err == nil {
					dispatcher.HandleGetVerResponse(st.imei, text)
					continue
				}
				lg.Warn("codec12: frame not parsed")
				continue
			}
			// (B2) AVL (Codec 0x08/0x8E): despachar y ACK dinámico (= Qty1)
			if codecID == 0x08 || codecID == 0x8E {
				if len(pkt) < 10 {
					lg.Warn("bad avl frame (len)", "len", len(pkt))
					continue
				}
				qty1 := int(pkt[9]) // Qty1 va justo después del CodecID
				go dispatcher.ProcessIncoming(st.imei, pkt)
				var ack [4]byte
				binary.BigEndian.PutUint32(ack[:], uint32(qty1))
				if _, err := conn.Write(ack[:]); err != nil {
					lg.Error("ack write failed", "err", err)
				} else {
					observability.RecordsAck.Inc()
					firstAVLACK = true
				}
				// (B3) Tras el primer ACK AVL, enviar getver una sola vez
				if st.ready && firstAVLACK && !st.sentGetVer {
					p := codec.BuildCodec12("getver")
					if _, err := conn.Write(p); err == nil {
						st.sentGetVer = true
						lg.Info("sent getver", "imei", st.imei)
					} else {
						lg.Warn("send getver failed", "err", err)
					}
				}
				continue
			}
			// (B4) Otros codecs: sólo log
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
