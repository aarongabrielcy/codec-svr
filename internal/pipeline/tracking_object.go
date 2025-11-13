package pipeline

type TrackingObject struct {
	IMEI     string `json:"imei"`
	Model    string `json:"model,omitempty"`
	FWVer    string `json:"fw_ver,omitempty"`
	Datetime string `json:"dt"`

	Lat  float64 `json:"lat"`
	Lon  float64 `json:"lon"`
	Spd  int     `json:"spd"`
	Crs  int     `json:"crs"`
	Sats int     `json:"sats"`

	PermIO map[string]uint64 `json:"perm_io"`

	MsgType int `json:"msg_type"` // 1=live, 0=buffer
	Fix     int `json:"fix"`      // 1 si sats>3 y coords válidas
}
