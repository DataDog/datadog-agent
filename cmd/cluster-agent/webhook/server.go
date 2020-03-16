// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

/*
Package api implements the agent IPC api. Using HTTP
calls, it's possible to communicate with the agent,
sending commands and receiving infos.
*/
package webhook

import (
	"crypto/tls"
	"fmt"
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

var port = 5004


// StartServer creates the router and starts the HTTP server
func StartServer(sc clusteragent.ServerContext) error {
	log.Error("Simplified server 4") // TODO remove me

	certFile := "/certs/cert.pem"// config.Datadog.GetString("cluster_agent.webhook.tls_cert_file")
	keyFile := "/certs/key.pem"//config.Datadog.GetString("cluster_agent.webhook.tls_key_file")
	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Errorf("Failed to load key pair: %v", err)
	}

	whsvr := &WebhookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":%v",  port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
	}

	// define http server and server handler
	log.Error("Listening on status endpoint")
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/status", whsvr.status)
	whsvr.server.Handler = mux

	// start webhook server in new routine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			log.Errorf("Failed to listen and serve webhook server: %v", err)
		}
	}()
	return nil
}
