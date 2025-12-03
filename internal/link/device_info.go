package link

// DeviceInfo es la vista "estática" del dispositivo que se envía a socket
type DeviceInfo struct {
	IMEI       string
	FWVer      string
	Model      string
	ICCID      string
	RemoteIP   string
	RemotePort int
	State      DeviceState
}
