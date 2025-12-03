package link

// DeviceState representa el tipo de evento del dispositivo
type DeviceState int

const (
	DeviceStateUnknown DeviceState = iota
	DeviceStateConnect             // device_connect: true
	DeviceStateUpdate              // device_update: true
)
