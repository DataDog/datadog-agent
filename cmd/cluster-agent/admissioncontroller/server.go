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
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var (
	server *http.Server
)

func getListener(port int) (net.Listener, error) {
	return net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
}

// StartServer creates the router and starts the HTTP server
func StartServer() error {
	certFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_cert_file")
	keyFile := config.Datadog.GetString("cluster_agent.admission_controller.tls_key_file")

	log.Debugf("loading TLS certificate: cert.pem %s, key.pem %s", certFile, keyFile)

	pair, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		log.Errorf("Failed to load key pair: %v", err)
	}

	port := config.Datadog.GetInt("cluster_agent.admissioncontroller_port")
	ln, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%v", port))
	if err != nil {
		log.Errorf("failed to create listener: %v", err)
	}

	log.Infof("Listening on admission controller endpoint, port %d", port)
	mux := http.NewServeMux()
	mux.HandleFunc("/mutate", serve)
	mux.HandleFunc("/status", status)

	conf := tls.Config{Certificates: []tls.Certificate{pair}}
	server := &http.Server{Handler: mux, TLSConfig: &conf}
	tlsln := tls.NewListener(ln, &conf)

	go server.Serve(tlsln)
	return nil
}

// StopServer closes the TLS server
func StopServer() {
	if server != nil {
		server.Close()
	}
}
