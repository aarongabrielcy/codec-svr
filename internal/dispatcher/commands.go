package dispatcher

import (
	"codec-svr/internal/store"
	"log/slog"
	"net"
	"strings"
	"sync"
	"time"
)

/* =======================================================================
                        COMMAND DEFINITION
======================================================================= */

type Command struct {
	Name             string
	Build            func() []byte
	Handler          func(imei, text string)
	DailyLimit       int
	SessionLimit     int
	MinRetryInterval time.Duration
	Condition        func(imei string) bool
}

var (
	cmdMu    sync.RWMutex
	registry = map[string]Command{}
)

func RegisterCommand(c Command) {
	cmdMu.Lock()
	defer cmdMu.Unlock()
	registry[c.Name] = c
}

func getCmd(name string) (Command, bool) {
	cmdMu.RLock()
	defer cmdMu.RUnlock()
	c, ok := registry[name]
	return c, ok
}

/* =======================================================================
                     PER-IMEI COMMAND SESSION STATE
======================================================================= */

type perCmdState struct {
	SessionCount int
	LastAttempt  time.Time
}

var (
	stateMu  sync.Mutex
	cmdState = make(map[string]map[string]*perCmdState)
)

func getState(imei, cmd string) *perCmdState {
	stateMu.Lock()
	defer stateMu.Unlock()

	if cmdState[imei] == nil {
		cmdState[imei] = make(map[string]*perCmdState)
	}

	st, ok := cmdState[imei][cmd]
	if !ok {
		st = &perCmdState{}
		cmdState[imei][cmd] = st
	}
	return st
}

/* =======================================================================
                     REQUIRED DATA CHECK
======================================================================= */

func needsToRun(imei, cmd string) bool {

	switch cmd {

	case "getver":
		fw := store.GetStringSafe("dev:" + imei + ":fw")
		model := store.GetStringSafe("dev:" + imei + ":model")
		return fw == "" || model == ""

	case "iccid_primary":
		return store.GetStringSafe("dev:"+imei+":iccid") == ""

	case "iccid_fallback":
		// solo si iccid sigue vacío
		return store.GetStringSafe("dev:"+imei+":iccid") == ""
	}

	return false
}

/* =======================================================================
                  UNIVERSAL COMMAND SCHEDULE FUNCTION
======================================================================= */

func TrySchedule(imei, cmdName string, conn net.Conn, lg *slog.Logger) {

	cmd, ok := getCmd(cmdName)
	if !ok {
		lg.Warn("unknown command", "cmd", cmdName)
		return
	}

	// Optional condition
	if cmd.Condition != nil && !cmd.Condition(imei) {
		return
	}

	// Should we run this command?
	if !needsToRun(imei, cmdName) {
		return
	}

	st := getState(imei, cmdName)
	now := time.Now()

	/* ---------------- session-limit ---------------- */
	if st.SessionCount >= cmd.SessionLimit {
		return
	}

	/* -------------- min retry interval -------------- */
	if !st.LastAttempt.IsZero() &&
		now.Sub(st.LastAttempt) < cmd.MinRetryInterval {
		return
	}

	/* -------------- daily limit via Redis ------------ */
	allowed, dailyCount, err := store.IncDailyCmdCounter(
		imei,
		cmdName,
		cmd.DailyLimit,
	)
	if err != nil || !allowed {
		return
	}

	/* --------------------- SEND --------------------- */
	frame := cmd.Build()
	if _, err := conn.Write(frame); err != nil {
		lg.Error("command send failed", "cmd", cmdName, "imei", imei, "err", err)
		return
	}

	st.SessionCount++
	st.LastAttempt = now

	lg.Info("command sent",
		"cmd", cmdName,
		"imei", imei,
		"session", st.SessionCount,
		"daily", dailyCount,
	)
}

/* =======================================================================
              UNIVERSAL ROUTER FOR COMMAND RESPONSES
======================================================================= */

func HandleCommandResponses(imei, text string) {

	lower := strings.ToLower(text)

	// GETVER
	if strings.Contains(lower, "ver:") ||
		strings.Contains(lower, "hw:") {
		HandleGetVerResponse(imei, text)
		return
	}

	// ICCID PRIMARY
	if strings.Contains(lower, "iccid") {
		HandleICCIDResponse(imei, text)
		return
	}

	// ICCID FALLBACK
	if strings.Contains(lower, "param values") {
		HandleICCIDResponse(imei, text)
		return
	}
}
