// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"net/http"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/api/coverage"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

var errNoIPCComponent = errors.New("cannot start the Datadog flare extension server: no IPC component provided, which is required to authenticate requests")

type server struct {
	srv      *http.Server
	listener net.Listener
}

func newServer(endpoint string, handler http.Handler, optIpcComp option.Option[ipc.Component]) (*server, error) {
	r := http.NewServeMux()
	r.Handle("/", handler)
	coverage.SetupCoverageHandler(r)

	s := &http.Server{
		Addr:    endpoint,
		Handler: r,
	}

	// The IPC component is mandatory: it supplies both the TLS server config and the
	// authentication middleware for this endpoint. Serving without it would expose the
	// Agent's effective configuration, environment and status to any local caller, so
	// fail closed rather than falling back to an unauthenticated listener.
	ipcComp, ok := optIpcComp.Get()
	if !ok {
		return nil, errNoIPCComponent
	}
	s.TLSConfig = ipcComp.GetTLSServerConfig()
	s.Handler = ipcComp.HTTPMiddleware(r)

	listener, err := net.Listen("tcp", endpoint)
	if err != nil {
		return nil, err
	}

	tlsListener := tls.NewListener(listener, s.TLSConfig)

	return &server{
		srv:      s,
		listener: tlsListener,
	}, nil

}

func (s *server) start() error {
	return s.srv.Serve(s.listener)
}

func (s *server) shutdown(ctx context.Context) error {
	if err := s.srv.Shutdown(ctx); err != nil {
		return err
	}
	// close `tlsListener` in case the server was never started.
	if err := s.listener.Close(); err != nil && !errors.Is(err, net.ErrClosed) {
		return err
	}
	return nil
}
