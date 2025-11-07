package server

import (
	"bytes"
	"encoding/binary"
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

		if st.ready && !st.sentGetVer {
			pkt := codec.BuildCodec12("getver")
			if _, err := conn.Write(pkt); err == nil {
				st.sentGetVer = true
				slog.Info("sent getver", "imei", st.imei)
			}
		}
		for {
			pkt := tryReadAVLFrame(&st.buf)
			if pkt == nil {
				break
			}
			observability.PacketsRecv.Inc()

			// ¿Es Codec 12 (comando/respuesta)?
			if len(pkt) >= 13 && pkt[8] == 0x0C {
				if text, err := codec.ParseCodec12Response(pkt); err == nil {
					// Manejar respuesta de comando (p.ej. getver) y NO enviar ACK
					dispatcher.HandleGetVerResponse(st.imei, text)
					continue
				}
			}
			// No es Codec12 -> trata como AVL normal
			go dispatcher.ProcessIncoming(st.imei, pkt)

			// ACK a Teltonika: por ahora 1 registro (siempre hay 1 en tus tramas)
			_ = binary.Write(conn, binary.BigEndian, uint32(1))
			observability.RecordsAck.Inc()
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
