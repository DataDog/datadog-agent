// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ipc

import (
	"crypto/x509"
	"fmt"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// Server is an abstraction of an IPC server, it provides features like internal DNS and multi listeners
type Server struct {
	listeners []net.Listener
	server    *http.Server
}

// ServerOptionCb allows configuration of the *http.Client during construction
type ServerOptionCb func(*serverParams)

type serverParams struct {
	resolver AddrResolver

	// if set to true the listener will contact sequencially the different endpoint until receiving a valid response (only if the resolver return multiple endpoints)
	withMultiListeners bool

	withStrictMode bool
}

// NewIPCServer returns an instance of Server
func NewIPCServer(server *http.Server, options ...ServerOptionCb) (Server, error) {
	listeners := []net.Listener{}
	params := serverParams{}

	for _, opt := range options {
		opt(&params)
	}

	if params.resolver == nil {
		params.resolver = NewConfigResolver()
	}

	if server.Addr == "" {
		return Server{}, fmt.Errorf("unable to create server: addr is empty")
	}

	endpoints, err := params.resolver.Resolve(server.Addr)
	if err != nil {
		return Server{}, fmt.Errorf("unable to create server for addr %v: %v", server.Addr, err.Error())
	}
	if len(endpoints) == 0 {
		return Server{}, fmt.Errorf("unable to start serve: no listeners provided")
	}

	if server.TLSConfig != nil {
		dnsCertExist := false
	out:
		for _, rawCert := range server.TLSConfig.Certificates {
			cert, _ := x509.ParseCertificate(rawCert.Certificate[0])
			for _, dnsname := range cert.DNSNames {
				if dnsname == server.Addr {
					dnsCertExist = true
					break out
				}
			}
		}
		if !dnsCertExist {
			log.Warnf("Provided TLS configuration without DNS name set, the connection might not work for server %v", server.Addr)
		}
	}

	for idx, endpoint := range endpoints {
		if idx > 0 && params.withMultiListeners {
			break
		}

		l, err := endpoint.Listener()
		if err != nil {
			log.Warnf("unable to listen on %v", endpoint.Addr())
			if params.withStrictMode {
				return Server{}, fmt.Errorf("unable to create server for addr %v: %v", server.Addr, err.Error())
			}
			continue
		}
		listeners = append(listeners, l)
	}

	if len(listeners) == 0 {
		return Server{}, fmt.Errorf("unable to create server for addr %v: no listeners available", server.Addr)
	}

	return Server{
		server:    server,
		listeners: listeners,
	}, nil
}

// WithMultiListeners activates the multi listener listening
func WithMultiListeners() func(r *serverParams) {
	return func(r *serverParams) {
		r.withMultiListeners = true
	}
}

// WithServerResolver replace the default Configuration resolver by a provided one
func WithServerResolver(resolver AddrResolver) func(r *serverParams) {
	return func(r *serverParams) {
		r.resolver = resolver
	}
}

// Serve call the underlying server.Serve() function with each of the listeners
func (s *Server) Serve() error {
	return s.serve(s.server.Serve)
}

// ServeTLS call the underlying server.ServeTLS() function with each of the listeners
func (s *Server) ServeTLS() error {
	return s.serve(func(l net.Listener) error {
		return s.server.ServeTLS(l, "", "")
	})
}

func (s *Server) serve(serveFunc func(l net.Listener) error) error {
	errChan := make(chan error, 2)

	for _, l := range s.listeners {
		go func() {
			errChan <- serveFunc(l)
		}()
		defer l.Close()
	}

	return <-errChan
}
