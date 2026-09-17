// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package api

import (
	"errors"
	"fmt"
	"net"
	"net/http"
	"strconv"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// getSIDForConnectionOwner is a var (not a direct call) so tests can stub the lookup without a real
// Windows connection.
var getSIDForConnectionOwner = procutil.GetSIDForConnectionOwner

// connectionOwnerSIDHandler returns the Windows SID of the process owning the loopback TCP connection
// described by the query parameters, as plain text. Callers pass the connection's 4-tuple rather than a
// pre-resolved PID: a PID is not a stable identifier, and resolving it here — where the owning handle is
// held and the ownership is re-validated (see procutil.GetSIDForConnectionOwner) — is what closes the
// PID-reuse race between a caller reading the TCP table and this lookup. This lets lower-privileged agent
// processes (e.g. the GUI, running as ddagentuser) resolve a peer's owning identity by asking
// process-agent, which runs as LocalSystem, instead of being granted that broad user right themselves.
//
// Query parameters (all required): family (4 or 6), laddr/lport (the peer's local endpoint),
// raddr/rport (the GUI server endpoint the peer connected to).
func connectionOwnerSIDHandler(w http.ResponseWriter, req *http.Request) {
	q := req.URL.Query()

	var family uint32
	switch q.Get("family") {
	case "4":
		family = windows.AF_INET
	case "6":
		family = windows.AF_INET6
	default:
		writeError(fmt.Errorf("invalid family %q (want 4 or 6)", q.Get("family")), http.StatusBadRequest, w)
		return
	}

	localAddr := net.ParseIP(q.Get("laddr"))
	remoteAddr := net.ParseIP(q.Get("raddr"))
	if localAddr == nil || remoteAddr == nil {
		writeError(errors.New("invalid or missing laddr/raddr"), http.StatusBadRequest, w)
		return
	}

	localPort, errL := strconv.Atoi(q.Get("lport"))
	remotePort, errR := strconv.Atoi(q.Get("rport"))
	if errL != nil || errR != nil || localPort <= 0 || localPort > 65535 || remotePort <= 0 || remotePort > 65535 {
		writeError(errors.New("invalid or missing lport/rport"), http.StatusBadRequest, w)
		return
	}

	sid, err := getSIDForConnectionOwner(family, localAddr, localPort, remoteAddr, remotePort)
	if err != nil {
		var errno windows.Errno
		switch {
		case errors.Is(err, procutil.ErrConnectionOwnerNotFound):
			writeError(err, http.StatusNotFound, w)
		case errors.Is(err, procutil.ErrConnectionOwnerChanged):
			writeError(err, http.StatusConflict, w)
		case errors.As(err, &errno) && errno == windows.ERROR_ACCESS_DENIED:
			writeError(err, http.StatusForbidden, w)
		default:
			writeError(err, http.StatusInternalServerError, w)
		}
		return
	}

	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write([]byte(sid))
}
