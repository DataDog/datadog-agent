// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddflareextensionimpl defines the OpenTelemetry Extension implementation.
package ddflareextensionimpl

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"net/http"

	"github.com/gorilla/mux"

	ipc "github.com/DataDog/datadog-agent/comp/core/ipc/def"
	pkgtoken "github.com/DataDog/datadog-agent/pkg/api/security"
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

	if ipcComp, ok := optIpcComp.Get(); ok {
		// Use the TLS configuration from the IPC component if available
		s.TLSConfig = ipcComp.GetTLSServerConfig()
		r.Use(ipcComp.HTTPMiddleware)
	} else {
		// Use generated self-signed certificate if running outside of the Agent
		tlsConfig, err := generateSelfSignedCert()
		if err != nil {
			return nil, err
		}
		s.TLSConfig = &tlsConfig
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

func generateSelfSignedCert() (tls.Config, error) {
	// create cert
	hosts := []string{"127.0.0.1", "localhost", "::1"}
	_, rootCertPEM, rootKey, err := pkgtoken.GenerateRootCert(hosts, 2048)
	if err != nil {
		return tls.Config{}, fmt.Errorf("unable to generate a self-signed certificate: %v", err)
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create a TLS cert using the private key and certificate
	rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		return tls.Config{}, fmt.Errorf("unable to generate a self-signed certificate: %v", err)

	}

	return tls.Config{
		Certificates: []tls.Certificate{rootTLSCert},
	}, nil
}
