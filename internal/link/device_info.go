package link

// DeviceInfo contiene la vista "estática" del dispositivo
// destinada a socket-tcp-proxy.
type DeviceInfo struct {
	IMEI       string
	FWVer      string
	Model      string
	ICCID      string
	RemoteIP   string
	RemotePort int
	State      DeviceState
}
