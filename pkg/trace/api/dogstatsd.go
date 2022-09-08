// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package api

import (
	"fmt"
	"io"
	"net"
	"net/http"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

// dogstatsdProxyHandler returns a new HTTP handler which will proxy requests to the DogStatsD
// endpoint in the Core Agent over UDP.
func (r *HTTPReceiver) dogstatsdProxyHandler() http.Handler {
	if !r.conf.StatsdEnabled {
		log.Info("DogstatsD disabled in the Agent configuration")
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "405 Method Not Allowed", http.StatusMethodNotAllowed)
		})
	}
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// TODO: Simplify this. We shouldn't need to open a new connection for every
		// request to the statsd endpoit. We should be able to just route the reverse
		// proxy to that endpoint.
		var network, address string
		if r.conf.StatsdSocket != "" {
			network, address = "unix", r.conf.StatsdSocket
		} else {
			network, address = "udp", fmt.Sprintf("%s:%d", r.conf.StatsdHost, r.conf.StatsdPort)
		}
		conn, err := net.Dial(network, address)
		if err != nil {
			log.Errorf("Error connecting to %s endpoint at %q: %v", network, address, err)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		defer conn.Close()
		if _, err := io.Copy(conn, req.Body); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}
