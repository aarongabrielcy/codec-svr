package codec

import "time"

type IOItem struct {
	Size int    `json:"size"`
	Val  uint64 `json:"val,omitempty"`
	Raw  []byte `json:"raw,omitempty"`
}

type GPSData struct {
	Longitude  float64 `json:"longitude"`
	Latitude   float64 `json:"latitude"`
	Altitude   int     `json:"altitude"`
	Angle      int     `json:"angle"`
	Satellites int     `json:"satellites"`
	Speed      int     `json:"speed"`
}

type AVLRecord struct {
	Timestamp time.Time         `json:"timestamp"`
	Priority  int               `json:"priority"`
	GPS       GPSData           `json:"gps"`
	EventIOID int               `json:"event_io_id"`
	TotalIO   int               `json:"total_io"`
	IO        map[uint16]IOItem `json:"io"`
}

type AvlPacket struct {
	Preamble uint32      `json:"preamble"`
	Len      uint32      `json:"data_len"`
	CodecID  uint8       `json:"codec_id"`
	Qty1     uint8       `json:"qty1"`
	Records  []AVLRecord `json:"records"`
	Qty2     uint8       `json:"qty2"`
	CRC      uint32      `json:"crc"`
}
