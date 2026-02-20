// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package hpflareextension defines the server for opentelemetry flare extensions.
package hpflareextension

import (
	"context"
	"crypto/tls"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
)

type server struct {
	srv      *http.Server
	listener net.Listener
}

func newServer(endpoint string, handler http.Handler, ipcComp ipc.Component) (*server, error) {
	r := mux.NewRouter()
	r.Handle("/", handler)

	s := &http.Server{
		Addr:    endpoint,
		Handler: r,
	}

	s.TLSConfig = ipcComp.GetTLSServerConfig()
	r.Use(ipcComp.HTTPMiddleware)

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
