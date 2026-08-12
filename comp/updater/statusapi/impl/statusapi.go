// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

// Package statusapiimpl implements the installer read-only status api component.
package statusapiimpl

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"time"

	compdef "github.com/DataDog/datadog-agent/comp/def"
	statusapi "github.com/DataDog/datadog-agent/comp/updater/statusapi/def"
	updatercomp "github.com/DataDog/datadog-agent/comp/updater/updater/def"
	"github.com/DataDog/datadog-agent/pkg/api/middleware"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// readTimeout bounds how long a client can take to send its request headers, so a
// connection that opens and then goes quiet cannot pin a goroutine.
const readTimeout = 10 * time.Second

// Requires defines the dependencies for the installer status api component.
type Requires struct {
	Lifecycle compdef.Lifecycle

	Updater updatercomp.Component
}

// Provides defines the output of the installer status api component.
type Provides struct {
	Comp statusapi.Component
}

// statusProvider is the slice of the daemon this listener is allowed to reach.
// Narrowing it here keeps the privileged parts of the daemon out of reach of a
// listener the Agent user can talk to.
type statusProvider interface {
	GetStatus() statusapi.Status
}

// server serves the daemon's read-only status to the Agent user.
//
// It is intentionally a second listener rather than a route on the daemon's local
// API: that API's other routes install, remove and promote packages as root, so its
// socket is 0700 and must stay that way. Only add non-sensitive, read-only routes
// here — in particular never serve secrets_pub_key or anything from
// GetRemoteConfigState.
type server struct {
	daemon   statusProvider
	listener net.Listener
	server   *http.Server
}

// NewComponent creates a new installer status api component.
//
// A listener that cannot be created is logged and swallowed rather than returned:
// this listener only feeds host metadata, while the daemon it belongs to applies
// remote upgrades. Failing here would trade a missing metadata payload for a host
// that no longer takes upgrades at all — and the socket lives in a directory the
// Agent user can write to, so an unprivileged process has some influence over
// whether the bind succeeds.
func NewComponent(reqs Requires) Provides {
	listener, err := listen()
	if err != nil {
		log.Errorf("Could not create the installer status API, installer metadata will report the installer as unreachable: %v", err)
		// Component is empty, so anything satisfies it: with no listener there is
		// nothing to run, and no lifecycle hook to register.
		return Provides{Comp: struct{}{}}
	}

	s := newServer(reqs.Updater, listener)
	reqs.Lifecycle.Append(compdef.Hook{OnStart: s.start, OnStop: s.stop})
	return Provides{Comp: s}
}

// newServer wraps an already-permissioned listener. The listener is what differs
// between platforms; everything below it does not.
func newServer(daemon statusProvider, listener net.Listener) *server {
	return &server{
		daemon:   daemon,
		listener: listener,
		server: &http.Server{
			ReadHeaderTimeout: readTimeout,
			IdleTimeout:       readTimeout,
		},
	}
}

func (s *server) start(_ context.Context) error {
	s.server.Handler = s.handler()
	go func() {
		err := s.server.Serve(s.listener)
		if err != nil {
			log.Infof("Installer status API server stopped: %v", err)
		}
	}()
	return nil
}

func (s *server) stop(ctx context.Context) error {
	return s.server.Shutdown(ctx)
}

func (s *server) handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /status", s.status)
	return middleware.RequireContentType("application/json")(mux)
}

// example: curl --unix-socket /opt/datadog-packages/run/installer-status.sock -H 'Content-Type: application/json' http://installer/status
func (s *server) status(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(s.daemon.GetStatus()); err != nil {
		log.Warnf("could not write installer status response: %v", err)
	}
}
