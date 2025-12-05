package link

import (
	"bufio"
	"codec-svr/internal/pipeline"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"
	"time"
)

/* -------------------------------------------------------
   VARIABLES GLOBALES
---------------------------------------------------------*/

var (
	proxyAddr string
	logger    *slog.Logger

	mu   sync.Mutex
	conn net.Conn

	// buffer SOLO para connect & update
	pending []interface{}
)

/* -------------------------------------------------------
   INIT
---------------------------------------------------------*/

func Init(addr string, lg *slog.Logger) {
	proxyAddr = addr
	if proxyAddr == "" {
		lg.Info("link: disabled (no proxy address configured)")
		return
	}
	logger = lg.With("component", "link")

	go connectLoop()
}

/* -------------------------------------------------------
   CONEXIÓN & RECONEXIÓN
---------------------------------------------------------*/

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

		// Enviar pendientes apenas se establezca la conexión
		flushPending()

		// Leer hasta que se caiga
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

/* -------------------------------------------------------
   PENDING BUFFER
---------------------------------------------------------*/

func addPending(v interface{}) {
	mu.Lock()
	defer mu.Unlock()
	pending = append(pending, v)
}

// Envía todos los mensajes pendientes cuando el link se conecta.
func flushPending() {
	mu.Lock()
	toSend := pending
	pending = nil
	mu.Unlock()

	for _, msg := range toSend {
		if err := sendNDJSON(msg); err != nil {
			// si falla otra vez → lo regresamos al pending
			addPending(msg)
			if logger != nil {
				logger.Warn("link: pending resend failed, re-buffered", "err", err)
			}
		}
	}
}

/* -------------------------------------------------------
   LECTURA DESDE PROXY
---------------------------------------------------------*/

func readLoop(c net.Conn) {
	sc := bufio.NewScanner(c)

	for sc.Scan() {
		line := sc.Bytes()
		handleIncoming(line)
	}

	if err := sc.Err(); err != nil && err != io.EOF {
		if logger != nil {
			logger.Warn("link: read error", "err", err)
		}
	}
}

func handleIncoming(line []byte) {
	if logger != nil {
		logger.Info("link: incoming", "line", string(line))
	}
	// en futuro: ruteo hacia dispatcher/server
}

/* -------------------------------------------------------
   ENVÍO NDJSON
---------------------------------------------------------*/

func sendNDJSON(v interface{}) error {
	c := getConn()
	if c == nil {
		addPending(v)
		return fmt.Errorf("link: not connected")
	}

	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	_, err = c.Write(append(b, '\n'))
	return err
}

/* -------------------------------------------------------
   PAYLOADS
---------------------------------------------------------*/

type deviceConnectPayload struct {
	DeviceConnect bool   `json:"device_connect"`
	IMEI          string `json:"imei"`
	FWVer         string `json:"fw_ver,omitempty"`
	Model         string `json:"model,omitempty"`
	ICCID         string `json:"iccid,omitempty"`
	RemoteIP      string `json:"remote_ip,omitempty"`
	RemotePort    int    `json:"remote_port,omitempty"`
}

type deviceUpdatePayload struct {
	DeviceUpdate bool   `json:"device_update"`
	IMEI         string `json:"imei"`
	FWVer        string `json:"fw_ver,omitempty"`
	Model        string `json:"model,omitempty"`
	ICCID        string `json:"iccid,omitempty"`
}

type trackingPayload = pipeline.TrackingObject

/* -------------------------------------------------------
   API PÚBLICA
---------------------------------------------------------*/

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

func SendTracking(tr *pipeline.TrackingObject) {
	if proxyAddr == "" || tr == nil {
		return
	}

	// IMPORTANTE: tracking NO se guarda en pending
	if err := sendNDJSON((*trackingPayload)(tr)); err != nil && logger != nil {
		logger.Warn("link: send tracking failed", "imei", tr.IMEI, "err", err)
	}
}
