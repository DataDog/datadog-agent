// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package admissioncontroller

import (
	"crypto/tls"
	"fmt"
	"github.com/DataDog/datadog-agent/pkg/config"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/clusteragent"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	version string
)

type WebhookServer struct {
	server *http.Server
}

// StartServer creates the router and starts the HTTP server
func StartServer(sc clusteragent.ServerContext) error {
	certFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_cert_file")
	keyFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_key_file")

	log.Infof("Loading TLS certificate: cert.pem %v, key.pem %v", certFile, keyFile)

	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Errorf("Failed to load key pair: %v", err)
	}

	whsvr := &WebhookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v",  config.Datadog.GetInt("cluster_agent.admissioncontroller_port")),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
	}

	// define http server and server handler
	log.Info("Listening on admission controller endpoint")
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/status", whsvr.status)
	whsvr.server.Handler = mux

	// start admission controller server in new routine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			log.Errorf("Failed to listen and serve admission controller server: %v", err)
		}
	}()
	return nil
}
