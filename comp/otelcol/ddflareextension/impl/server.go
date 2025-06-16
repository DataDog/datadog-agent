// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	"github.com/DataDog/datadog-agent/pkg/util/option"
)

type server struct {
	srv      *http.Server
	listener net.Listener
}

func newServer(endpoint string, handler http.Handler, optIpcComp option.Option[ipc.Component]) (*server, error) {
	r := mux.NewRouter()
	r.Handle("/", handler)

	s := &http.Server{
		Addr:    endpoint,
		Handler: r,
	}

	// no easy way currently to pass required bearer auth token to OSS collector;
	// skip the validation if running inside a separate collector
	// TODO: determine way to allow OSS collector to authenticate with agent, OTEL-2226
	if ipcComp, ok := optIpcComp.Get(); ok {
		// Use the TLS configuration from the IPC component if available
		s.TLSConfig = ipcComp.GetTLSServerConfig()
		r.Use(ipcComp.HTTPMiddleware)
	}

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
	return s.srv.Shutdown(ctx)
}
