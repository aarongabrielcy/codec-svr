package dispatcher

import (
	"fmt"
	"regexp"
	"strings"

	"codec-svr/internal/link"
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

	oldFW := store.GetStringSafe("dev:" + imei + ":fw")
	oldModel := store.GetStringSafe("dev:" + imei + ":model")

	// Guardar sólo si cambiaron
	changed := false

	if dv.Firmware != "" && dv.Firmware != oldFW {
		store.SaveStringSafe("dev:"+imei+":fw", dv.Firmware)
		changed = true
	}
	if dv.Model != "" && dv.Model != oldModel {
		store.SaveStringSafe("dev:"+imei+":model", dv.Model)
		changed = true
	}

	// Siempre mantener el raw original de GETVER
	store.SaveStringSafe("dev:"+imei+":getver_raw", dv.Raw)
	// Notificar al proxy sólo si hubo cambio relevante
	if changed {
		// Notificar al proxy que cambió el estado del dispositivo
		link.SendDeviceUpdate(link.DeviceInfo{
			IMEI:  imei,
			FWVer: dv.Firmware,
			Model: dv.Model,
			Brand: "TTKA",
			ICCID: store.GetStringSafe("dev:" + imei + ":iccid"),
		})
	}
	fmt.Printf("[GETVER] imei=%s model=%s fw=%s\n",
		dv.IMEI, dv.Model, dv.Firmware)

	return dv
}

func GetCachedModel(imei string) string {
	return store.GetStringSafe("dev:" + imei + ":model")
}
