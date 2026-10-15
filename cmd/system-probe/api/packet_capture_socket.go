// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package api

import (
	"errors"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/system-probe/api/server"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// packetCaptureRoutePrefix is where module.Register mounts the packet_capture
// module's routes.
const packetCaptureRoutePrefix = "/packet_capture/"

// newPacketCaptureHandler returns a handler that serves only the
// packet_capture module's routes from mux and returns 404 for everything else.
func newPacketCaptureHandler(mux http.Handler) http.Handler {
	captureMux := http.NewServeMux()
	captureMux.Handle(packetCaptureRoutePrefix, mux)
	return captureMux
}

// startPacketCaptureServer serves the packet_capture module on its own
// socket, so a caller that only triggers captures does not need the main
// system-probe socket. A failure is logged rather than returned: the other
// modules keep serving on the main socket.
func startPacketCaptureServer(socketPath string, mux http.Handler) {
	conn, err := server.NewListener(socketPath)
	if err != nil {
		log.Errorf("packet_capture: creating socket %s: %s", socketPath, err)
		return
	}
	go func() {
		err := http.Serve(conn, newPacketCaptureHandler(mux))
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("packet_capture: serving socket %s: %s", socketPath, err)
		}
	}()
}
