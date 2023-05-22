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
	"crypto/x509"
	"encoding/pem"
	"fmt"
	stdLog "log"
	"net"
	"net/http"
	"time"

	"github.com/cihub/seelog"
	"github.com/gorilla/mux"

	"github.com/DataDog/datadog-agent/cmd/security-agent/api/agent"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/compliance"
	"github.com/DataDog/datadog-agent/pkg/config"
	secagent "github.com/DataDog/datadog-agent/pkg/security/agent"
)

// Server implements security agent API server
type Server struct {
	listener net.Listener
	agent    *agent.Agent
}

// NewServer creates a new Server instance
func NewServer(runtimeAgent *secagent.RuntimeSecurityAgent, complianceAgent *compliance.Agent) (*Server, error) {
	listener, err := newListener()
	if err != nil {
		return nil, err
	}
	return &Server{
		listener: listener,
		agent:    agent.NewAgent(runtimeAgent, complianceAgent),
	}, nil
}

// Start creates the router and starts the HTTP server
func (s *Server) Start() error {
	// create the root HTTP router
	r := mux.NewRouter()

	// IPC REST API server
	s.agent.SetupHandlers(r.PathPrefix("/agent").Subrouter())

	// Validate token for every request
	r.Use(validateToken)

	err := util.CreateAndSetAuthToken()
	if err != nil {
		return err
	}

	hosts := []string{"127.0.0.1", "localhost"}
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return fmt.Errorf("unable to start TLS server")
	}

	// PEM encode the private key
	rootKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(rootKey),
	})

	// Create a TLS cert using the private key and certificate
	rootTLSCert, err := tls.X509KeyPair(rootCertPEM, rootKeyPEM)
	if err != nil {
		return fmt.Errorf("invalid key pair: %v", err)
	}

	tlsConfig := tls.Config{
		Certificates: []tls.Certificate{rootTLSCert},
		MinVersion:   tls.VersionTLS13,
	}

	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := config.NewLogWriter(4, seelog.ErrorLvl)

	srv := &http.Server{
		Handler:      r,
		ErrorLog:     stdLog.New(logWriter, "Error from the agent http API server: ", 0), // log errors to seelog,
		TLSConfig:    &tlsConfig,
		WriteTimeout: config.Datadog.GetDuration("server_timeout") * time.Second,
	}
	tlsListener := tls.NewListener(s.listener, &tlsConfig)

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

func validateToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.Validate(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}
