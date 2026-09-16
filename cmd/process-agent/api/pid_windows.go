// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build windows

package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
)

// getSIDForPID is a var (not a direct call) so tests can stub process lookup without a real Windows process.
var getSIDForPID = procutil.GetSIDForPID

// sidForPIDHandler returns the Windows SID of the process owning the {pid} path parameter, as plain text. This lets lower-privileged agent processes (e.g. the GUI, running as ddagentuser) resolve another process's owning identity by asking process-agent — which runs as LocalSystem and so already holds SeDebugPrivilege — instead of being granted that broad user right themselves.
func sidForPIDHandler(w http.ResponseWriter, req *http.Request) {
	pidStr := req.PathValue("pid")
	pid, err := strconv.ParseInt(pidStr, 10, 32)
	if err != nil || pid <= 0 {
		writeError(fmt.Errorf("invalid pid %q", pidStr), http.StatusBadRequest, w)
		return
	}

	sid, err := getSIDForPID(int32(pid))
	if err != nil {
		var errno windows.Errno
		switch {
		case errors.As(err, &errno) && errno == windows.ERROR_INVALID_PARAMETER:
			writeError(err, http.StatusNotFound, w)
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
