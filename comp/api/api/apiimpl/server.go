// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package apiimpl

import (
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	stdLog "log"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/listener"
	"github.com/DataDog/datadog-agent/comp/api/api/apiimpl/observability"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	pkglogsetup "github.com/DataDog/datadog-agent/pkg/util/log/setup"
)

func startServer(listener net.Listener, srv *http.Server, name string) {
	// Use a stack depth of 4 on top of the default one to get a relevant filename in the stdlib
	logWriter, _ := pkglogsetup.NewTLSHandshakeErrorWriter(5, log.ErrorLvl)

	srv.ErrorLog = stdLog.New(logWriter, fmt.Sprintf("Error from the Agent HTTP server '%s': ", name), 0) // log errors to seelog

	tlsListener := tls.NewListener(listener, srv.TLSConfig)

	go srv.Serve(tlsListener) //nolint:errcheck

	log.Infof("Started HTTP server '%s' on %s", name, listener.Addr().String())
}

func stopServer(listener net.Listener, name string) {
	if listener != nil {
		if err := listener.Close(); err != nil {
			log.Errorf("Error stopping HTTP server '%s': %s", name, err)
		} else {
			log.Infof("Stopped HTTP server '%s'", name)
		}
	}
}

// StartServers creates certificates and starts API + IPC servers
func (server *apiServer) startServers() error {
	apiAddr, err := listener.GetIPCAddressPort()
	if err != nil {
		return fmt.Errorf("unable to get IPC address and port: %v", err)
	}

	authTagGetter, err := authTagGetter(server.ipc.GetTLSServerConfig())
	if err != nil {
		return fmt.Errorf("unable to load the IPC certificate: %v", err)
	}

	// create the telemetry middleware
	tmf := observability.NewTelemetryMiddlewareFactory(server.telemetry, authTagGetter)

	// start the CMD server
	if err := server.startCMDServer(
		apiAddr,
		tmf,
	); err != nil {
		return fmt.Errorf("unable to start CMD API server: %v", err)
	}

	// start the IPC server
	if ipcServerPath, enabled := listener.GetIPCServerPath(); enabled {
		if err := server.startIPCServer(ipcServerPath, tmf); err != nil {
			// if we fail to start the IPC server, we should stop the CMD server
			server.stopServers()
			return fmt.Errorf("unable to start IPC API server: %v", err)
		}
	}

	return nil
}

// StopServers closes the connections and the servers
func (server *apiServer) stopServers() {
	stopServer(server.cmdListener, cmdServerName)
	stopServer(server.ipcListener, ipcServerName)
}

// authTagGetter returns a function that returns the auth tag for the given request
// It returns "mTLS" if the client provides a valid certificate, "token" otherwise
func authTagGetter(serverTLSConfig *tls.Config) (func(r *http.Request) string, error) {
	// Read the IPC certificate from the server TLS config
	if serverTLSConfig == nil || len(serverTLSConfig.Certificates) == 0 || len(serverTLSConfig.Certificates[0].Certificate) == 0 {
		return nil, errors.New("no certificates found in server TLS config")
	}

	cert, err := x509.ParseCertificate(serverTLSConfig.Certificates[0].Certificate[0])
	if err != nil {
		return nil, fmt.Errorf("error parsing IPC certificate: %v", err)
	}

	return func(r *http.Request) string {
		if r.TLS != nil && len(r.TLS.PeerCertificates) > 0 && cert.Equal(r.TLS.PeerCertificates[0]) {
			return "mTLS"
		}
		// We can assert that the auth is at least a token because it has been checked previously by the validateToken middleware
		return "token"
	}, nil
}
