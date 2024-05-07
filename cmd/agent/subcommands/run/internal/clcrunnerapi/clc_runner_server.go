// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

/*
Package clcrunnerapi implements the clc runner IPC api. Using HTTP
calls, the cluster Agent collects stats to optimize the cluster level checks dispatching.
*/
package clcrunnerapi

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

	v1 "github.com/DataDog/datadog-agent/cmd/agent/subcommands/run/internal/clcrunnerapi/v1"
	"github.com/DataDog/datadog-agent/comp/core/autodiscovery"
	"github.com/DataDog/datadog-agent/pkg/api/security"
	"github.com/DataDog/datadog-agent/pkg/api/util"
	"github.com/DataDog/datadog-agent/pkg/config"
)

var clcListener net.Listener

// StartCLCRunnerServer creates the router and starts the HTTP server
func StartCLCRunnerServer(extraHandlers map[string]http.Handler, ac autodiscovery.Component) error {
	// create the root HTTP router
	r := mux.NewRouter()

	// IPC REST API server
	v1.SetupHandlers(r.PathPrefix("/api/v1").Subrouter(), ac)

	// Register extra hanlders
	for path, handler := range extraHandlers {
		r.Handle(path, handler)
	}

	// Validate token for every request
	r.Use(validateCLCRunnerToken)

	// get the transport we're going to use under HTTP
	var err error
	clcListener, err = getCLCRunnerListener()
	if err != nil {
		return fmt.Errorf("unable to create the clc runner api server: %v", err)
	}

	// CLC Runner token
	// Use the Cluster Agent token
	err = util.InitDCAAuthToken(config.Datadog)
	if err != nil {
		return err
	}

	hosts := []string{"127.0.0.1", "localhost", config.Datadog.GetString("clc_runner_host")}
	_, rootCertPEM, rootKey, err := security.GenerateRootCert(hosts, 2048)
	if err != nil {
		return fmt.Errorf("unable to start TLS server: %v", err)
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
	logWriter, _ := config.NewLogWriter(4, seelog.WarnLvl)

	srv := &http.Server{
		Handler:           r,
		ErrorLog:          stdLog.New(logWriter, "Error from the clc runner http API server: ", 0), // log errors to seelog,
		TLSConfig:         &tlsConfig,
		WriteTimeout:      config.Datadog.GetDuration("clc_runner_server_write_timeout") * time.Second,
		ReadHeaderTimeout: config.Datadog.GetDuration("clc_runner_server_readheader_timeout") * time.Second,
	}
	tlsListener := tls.NewListener(clcListener, &tlsConfig)

	go srv.Serve(tlsListener) //nolint:errcheck
	return nil
}

// StopCLCRunnerServer closes the connection and the server
// stops listening to cluster agent queries.
func StopCLCRunnerServer() {
	if clcListener != nil {
		clcListener.Close()
	}
}

func validateCLCRunnerToken(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := util.ValidateDCARequest(w, r); err != nil {
			return
		}
		next.ServeHTTP(w, r)
	})
}
