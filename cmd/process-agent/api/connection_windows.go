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

// getSIDForConnectionOwner is a var so tests can stub the lookup without a real Windows connection.
var getSIDForConnectionOwner = procutil.GetSIDForConnectionOwner

// connectionOwnerSIDHandler returns the SID owning the loopback TCP 4-tuple (family, laddr/lport, raddr/rport, all required) as plain text; taking the 4-tuple and re-validating ownership here closes the PID-reuse race and lets low-privilege agents (GUI as ddagentuser) resolve a peer's identity via process-agent (LocalSystem).
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
