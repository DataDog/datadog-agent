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

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
)

type server struct {
	srv      *http.Server
	listener net.Listener
}

// agentConfigReader is the subset of config.Component the server needs.
type agentConfigReader interface {
	GetBool(key string) bool
}

func newServer(endpoint string, handler http.Handler, agentConfig agentConfigReader, ipcComp ipc.Component) (*server, error) {
	r := http.NewServeMux()
	r.Handle("/", handler)

	s := &http.Server{
		Addr:    endpoint,
		Handler: ipcComp.HTTPMiddleware(r),
	}

	// mTLS: clients must present the IPC certificate
	s.TLSConfig = ipcComp.GetTLSServerConfig()
	if !agentConfig.GetBool("insecure_temporarily_disable_sub_agent_ipc_mtls") {
		s.TLSConfig.ClientAuth = tls.RequireAndVerifyClientCert
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
