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
	t := strings.TrimSpace(text)

	dv := DeviceVersion{
		IMEI: imei,
		Raw:  t,
	}

	if m := reVer.FindStringSubmatch(t); len(m) > 1 {
		dv.Firmware = strings.TrimSpace(m[1])
	}
	if m := reHw.FindStringSubmatch(t); len(m) > 1 {
		dv.Model = strings.TrimSpace(m[1])
	}
	if m := reIMEI.FindStringSubmatch(t); len(m) > 1 {
		dv.IMEI = strings.TrimSpace(m[1])
	}

	// Save only if different
	if dv.Firmware != "" &&
		dv.Firmware != store.GetStringSafe("dev:"+imei+":fw") {
		store.SaveStringSafe("dev:"+imei+":fw", dv.Firmware)
	}

	if dv.Model != "" &&
		dv.Model != store.GetStringSafe("dev:"+imei+":model") {
		store.SaveStringSafe("dev:"+imei+":model", dv.Model)
	}

	lt := strings.ToLower(dv.Raw)
	if strings.Contains(lt, "ver:") && strings.Contains(lt, "hw:") {
		store.SaveStringSafe("dev:"+imei+":getver_raw", dv.Raw)
	}

	fmt.Printf("[GETVER] imei=%s model=%s fw=%s\n",
		dv.IMEI, dv.Model, dv.Firmware)

	return dv
}

func GetCachedModel(imei string) string {
	return store.GetStringSafe("dev:" + imei + ":model")
}
