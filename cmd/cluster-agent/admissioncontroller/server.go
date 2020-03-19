// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// Package admissioncontroller implements the agent admission webhook
// endpoint. This server receives k8s objects from the cluster and
// preforms changes or admission control to such objects.
//
// More info: https://kubernetes.io/docs/reference/access-authn-authz/admission-controllers/
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

// WebhookServer represents the http server for our admission controller
type WebhookServer struct {
	server *http.Server
}

// StartServer creates the router and starts the HTTP server
// TODO Also I don't think providing the clusteragent.ServerContext as parameter is useful here. Providing the mainCtx context.Context can be more useful to have gracefull stop of the webhook
// TODO I think it is not useful to use the sc clusteragent.ServerContext here, but instead we can use a ctx context.Context. thank to context.Context it will be possible to close the listener when the channel behind the ctx.Done() is closed
func StartServer(sc clusteragent.ServerContext) error {
	certFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_cert_file")
	keyFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_key_file")

	log.Debugf("loading TLS certificate: cert.pem %s, key.pem %s", certFile, keyFile)

	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Errorf("failed to load key pair: %v", err)
	}

	port := config.Datadog.GetInt("cluster_agent.admissioncontroller_port")
	whsvr := &WebhookServer{
		server: &http.Server{
			Addr:      fmt.Sprintf(":%d", port),
			TLSConfig: &tls.Config{Certificates: []tls.Certificate{pair}},
		},
	}

	// define http server and server handler
	log.Infof("listening on admission controller endpoint, port %d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", whsvr.serve)
	mux.HandleFunc("/status", whsvr.status)
	whsvr.server.Handler = mux

	// start admission controller server in new routine
	go func() {
		if err := whsvr.server.ListenAndServeTLS("", ""); err != nil {
			log.Errorf("failed to listen and serve admission controller server: %v", err)
		}
	}()
	return nil
}
