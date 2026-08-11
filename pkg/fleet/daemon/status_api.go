// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package daemon

import (
	"context"
	"encoding/json"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/api/middleware"
	daemonstatus "github.com/DataDog/datadog-agent/pkg/fleet/daemon/status"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// StatusAPI is the read-only API the daemon exposes to the Agent user.
//
// It is intentionally a second listener rather than a route on LocalAPI: the
// local API's other routes install, remove and promote packages as root, so its
// socket is 0700 and must stay that way. Only add non-sensitive, read-only
// routes here — in particular never serve secrets_pub_key or anything from
// GetRemoteConfigState.
type StatusAPI interface {
	Start(context.Context) error
	Stop(context.Context) error
}

// statusProvider is the slice of the daemon the status API is allowed to reach.
// Narrowing it here keeps the privileged parts of Daemon out of reach of a
// listener the Agent user can talk to.
type statusProvider interface {
	GetStatus() daemonstatus.Response
}

type statusAPIImpl struct {
	daemon   statusProvider
	listener net.Listener
	server   *http.Server
}

// Start starts the StatusAPI.
func (s *statusAPIImpl) Start(_ context.Context) error {
	s.server.Handler = s.handler()
	go func() {
		err := s.server.Serve(s.listener)
		if err != nil {
			log.Infof("Installer status API server stopped: %v", err)
		}
	}()
	return nil
}

// Stop stops the StatusAPI.
func (s *statusAPIImpl) Stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *statusAPIImpl) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.status)
	return middleware.RequireContentType("application/json")(mux)
}

// example: curl --unix-socket /opt/datadog-packages/run/installer-status.sock -H 'Content-Type: application/json' http://installer/status
func (s *statusAPIImpl) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.daemon.GetStatus()); err != nil {
		log.Warnf("could not write installer status response: %v", err)
	}
}
