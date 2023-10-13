// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

package api

import (
	stdlog "log"
	"net/http"
	"net/http/httputil"
	"time"

	"github.com/DataDog/datadog-agent/pkg/trace/log"
)

const (
	serverlessFlareEndpointPath = "/api/ui/support/serverless/flare"
)

type tracerFlareTransport struct {
	rt   http.RoundTripper
	path string
}

func (m *tracerFlareTransport) RoundTrip(req *http.Request) (rresp *http.Response, rerr error) {
	req.URL.Path = m.path
	return m.rt.RoundTrip(req)
}

func (r *HTTPReceiver) tracerFlareHandler() http.Handler {
	apiKey := r.conf.APIKey()

	director := func(req *http.Request) {
		req.Header.Set("DD-API-KEY", apiKey)
	}

	logger := log.NewThrottled(5, 10*time.Second) // limit to 5 messages every 10 seconds
	transport := r.conf.NewHTTPTransport()
	return &httputil.ReverseProxy{
		Director:  director,
		ErrorLog:  stdlog.New(logger, "tracer_flare.Proxy: ", 0),
		Transport: &tracerFlareTransport{transport, serverlessFlareEndpointPath},
	}
}
