package dispatcher

import (
	"fmt"
	"regexp"
	"strings"

	"codec-svr/internal/store"
)

var (
	reVer  = regexp.MustCompile(`(?i)\bver:([^\s]+(?:\s+Rev:?\s*\d+)?)`)
	reHw   = regexp.MustCompile(`(?i)\bhw:([A-Za-z0-9_-]+)`)
	reIMEI = regexp.MustCompile(`(?i)\bimei:([0-9]{14,17})`)
)

type DeviceVersion struct {
	IMEI     string
	Model    string
	Firmware string
	Raw      string
}

func HandleGetVerResponse(imei, text string) DeviceVersion {
	ver := ""
	hw := ""
	ime := imei

	if m := reVer.FindStringSubmatch(text); len(m) > 1 {
		ver = strings.TrimSpace(m[1])
	}
	if m := reHw.FindStringSubmatch(text); len(m) > 1 {
		hw = strings.TrimSpace(m[1])
	}
	if m := reIMEI.FindStringSubmatch(text); len(m) > 1 {
		ime = strings.TrimSpace(m[1])
	}

	dv := DeviceVersion{
		IMEI: ime, Model: hw, Firmware: ver, Raw: text,
	}
	fmt.Printf("[GETVER] imei=%s model=%s fw=%s raw=%q\n", dv.IMEI, dv.Model, dv.Firmware, dv.Raw)

	// Guarda algo útil en Redis (clave por IMEI)
	if dv.Firmware != "" {
		store.SaveStringSafe("dev:"+dv.IMEI+":fw", dv.Firmware)
	}
	if dv.Model != "" {
		store.SaveStringSafe("dev:"+dv.IMEI+":model", dv.Model)
	}
	store.SaveStringSafe("dev:"+dv.IMEI+":getver_raw", dv.Raw)

	return dv
}
