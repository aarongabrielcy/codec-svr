package link

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"

	"codec-svr/internal/pipeline"
)

// Configuración del link
var (
	proxyAddr string
	logger    *slog.Logger

	mu   sync.Mutex
	conn net.Conn
)

// Init arranca el cliente TCP hacia socket-tcp-proxy.
// Si addr == "", deja el link deshabilitado.
func Init(addr string, lg *slog.Logger) {
	proxyAddr = addr
	if proxyAddr == "" {
		lg.Info("link: disabled (no proxy address configured)")
		return
	}
	logger = lg.With("component", "link")

	go connectLoop()
}

// -------------------------------------------------------------------
//                        LOOP DE CONEXIÓN
// -------------------------------------------------------------------

func connectLoop() {
	for {
		c, err := net.Dial("tcp", proxyAddr)
		if err != nil {
			if logger != nil {
				logger.Error("link: dial failed", "addr", proxyAddr, "err", err)
			}
			time.Sleep(5 * time.Second)
			continue
		}

		setConn(c)
		if logger != nil {
			logger.Info("link: connected", "remote", c.RemoteAddr().String())
		}

		// leer en este hilo hasta que se caiga
		readLoop(c)

		clearConn(c)
		if logger != nil {
			logger.Warn("link: connection closed, reconnecting...")
		}
		time.Sleep(2 * time.Second)
	}
}

func setConn(c net.Conn) {
	mu.Lock()
	defer mu.Unlock()
	conn = c
}

func clearConn(c net.Conn) {
	mu.Lock()
	defer mu.Unlock()
	if conn == c {
		_ = conn.Close()
		conn = nil
	}
}

func getConn() net.Conn {
	mu.Lock()
	defer mu.Unlock()
	return conn
}

// -------------------------------------------------------------------
//                           LECTURA
// -------------------------------------------------------------------

func readLoop(c net.Conn) {
	r := bufio.NewScanner(c)
	for r.Scan() {
		line := r.Bytes()
		handleIncomingLine(line)
	}
	if err := r.Err(); err != nil && err != io.EOF {
		if logger != nil {
			logger.Warn("link: read error", "err", err)
		}
	}
}

// Por ahora sólo logueamos lo que llega del proxy.
// Más adelante aquí puedes rutear comandos hacia dispatcher / server.
func handleIncomingLine(line []byte) {
	if logger != nil {
		logger.Info("link: incoming line", "line", string(line))
	}
}

// -------------------------------------------------------------------
//                          ENVÍO NDJSON
// -------------------------------------------------------------------

func sendNDJSON(v interface{}) error {
	c := getConn()
	if c == nil {
		return fmt.Errorf("link: not connected")
	}
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	_, err = c.Write(append(b, '\n'))
	return err
}

// -------------------------------------------------------------------
//          PAYLOADS DE ALTO NIVEL HACIA EL PROXY (NDJSON)
// -------------------------------------------------------------------

// device_connect
type deviceConnectPayload struct {
	DeviceConnect bool   `json:"device_connect"`
	IMEI          string `json:"imei"`
	FWVer         string `json:"fw_ver,omitempty"`
	Model         string `json:"model,omitempty"`
	ICCID         string `json:"iccid,omitempty"`
	RemoteIP      string `json:"remote_ip,omitempty"`
	RemotePort    int    `json:"remote_port,omitempty"`
}

// device_update
type deviceUpdatePayload struct {
	DeviceUpdate bool   `json:"device_update"`
	IMEI         string `json:"imei"`
	FWVer        string `json:"fw_ver,omitempty"`
	Model        string `json:"model,omitempty"`
	ICCID        string `json:"iccid,omitempty"`
}

// tracking (reutilizamos TrackingObject y su formato JSON actual)
type trackingPayload = pipeline.TrackingObject

// -------------------------------------------------------------------
//                 FUNCIONES PÚBLICAS PARA EL RESTO
// -------------------------------------------------------------------

// SendDeviceConnect se llama normalmente tras el handshake TCP con el GPS.
func SendDeviceConnect(info DeviceInfo) {
	if proxyAddr == "" {
		return
	}
	pl := deviceConnectPayload{
		DeviceConnect: true,
		IMEI:          info.IMEI,
		FWVer:         info.FWVer,
		Model:         info.Model,
		ICCID:         info.ICCID,
		RemoteIP:      info.RemoteIP,
		RemotePort:    info.RemotePort,
	}
	if err := sendNDJSON(pl); err != nil && logger != nil {
		logger.Warn("link: send device_connect failed", "imei", info.IMEI, "err", err)
	}
}

// SendDeviceUpdate se llama cuando cambian fw/model/iccid.
func SendDeviceUpdate(info DeviceInfo) {
	if proxyAddr == "" {
		return
	}
	pl := deviceUpdatePayload{
		DeviceUpdate: true,
		IMEI:         info.IMEI,
		FWVer:        info.FWVer,
		Model:        info.Model,
		ICCID:        info.ICCID,
	}
	if err := sendNDJSON(pl); err != nil && logger != nil {
		logger.Warn("link: send device_update failed", "imei", info.IMEI, "err", err)
	}
}

// SendTracking envía el trackeo actual como NDJSON (formato TrackingObject).
func SendTracking(tr *pipeline.TrackingObject) {
	if proxyAddr == "" || tr == nil {
		return
	}
	if err := sendNDJSON((*trackingPayload)(tr)); err != nil && logger != nil {
		logger.Warn("link: send tracking failed", "imei", tr.IMEI, "err", err)
	}
}
