// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package api

import (
	"crypto/tls"
	stdLog "log"
	"net"
	"net/http"
	"time"

	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/security-agent/api/agent"
	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/comp/core/secrets"
	"github.com/DataDog/datadog-agent/comp/core/settings"
	"github.com/DataDog/datadog-agent/comp/core/status"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

// Server implements security agent API server
type Server struct {
	listener       net.Listener
	agent          *agent.Agent
	tlsConfig      *tls.Config
	authMiddleware mux.MiddlewareFunc
}

// NewServer creates a new Server instance
func NewServer(statusComponent status.Component, settings settings.Component, wmeta workloadmeta.Component, ipc ipc.Component, secrets secrets.Component) (*Server, error) {
	listener, err := newListener()
	if err != nil {
		return nil, err
	}
	return &Server{
		listener:       listener,
		agent:          agent.NewAgent(statusComponent, settings, wmeta, secrets),
		tlsConfig:      ipc.GetTLSServerConfig(),
		authMiddleware: ipc.HTTPMiddleware,
	}, nil
}

// Start creates the router and starts the HTTP server
func (s *Server) Start() error {
	// create the root HTTP router
	r := mux.NewRouter()

	// IPC REST API server
	s.agent.SetupHandlers(r.PathPrefix("/agent").Subrouter())

	// Validate token for every request
	r.Use(s.authMiddleware)

	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := pkglogsetup.NewLogWriter(4, log.ErrorLvl)

	srv := &http.Server{
		Handler:      r,
		ErrorLog:     stdLog.New(logWriter, "Error from the agent http API server: ", 0), // log errors to seelog,
		TLSConfig:    s.tlsConfig,
		WriteTimeout: pkgconfigsetup.Datadog().GetDuration("server_timeout") * time.Second,
	}
	tlsListener := tls.NewListener(s.listener, s.tlsConfig)

	go srv.Serve(tlsListener) //nolint:errcheck
	return nil
}

// Stop closes the connection and the server
// stops listening to new commands.
func (s *Server) Stop() {
	if s.listener != nil {
		s.listener.Close()
	}
}

// Address retruns the server address.
func (s *Server) Address() *net.TCPAddr {
	return s.listener.Addr().(*net.TCPAddr)
}
