package pipeline

// TrackingObject representa el paquete de datos normalizado que se enviará vía gRPC o log JSON.
type TrackingObject struct {
	IMEI       string            `json:"imei"`
	Datetime   string            `json:"datetime"`
	Latitude   float64           `json:"latitud"`
	Longitude  float64           `json:"longitud"`
	Speed      int               `json:"speed"`
	Course     int               `json:"course"`
	Satellites int               `json:"satellites"`
	Fix        int               `json:"fix"`
	Inputs     map[string]int    `json:"inputs"`
	Outputs    map[string]int    `json:"outputs"`
	Extras     map[string]uint64 `json:"extras"`

	// Campos adicionales
	MsgType string `json:"msg_type"`
	Model   string `json:"model,omitempty"`
	FWVer   string `json:"fw_ver,omitempty"`
}
