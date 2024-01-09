// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"

	"github.com/DataDog/datadog-agent/pkg/serverless/invocationlifecycle"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type runtimeProxy struct {
	target    *url.URL
	proxy     *httputil.ReverseProxy
	processor invocationlifecycle.InvocationProcessor
}

// Start starts the proxy
// This proxy allows us to inspect traffic from/to the AWS Lambda Runtime API
func Start(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) {
	go setup(proxyHostPort, originalRuntimeHostPort, processor)
}

func setup(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) {
	log.Debugf("runtime api proxy: starting reverse proxy on %s and forwarding to %s", proxyHostPort, originalRuntimeHostPort)
	proxy := newProxy(originalRuntimeHostPort, processor)

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.handle)

	s := &http.Server{
		Addr:    proxyHostPort,
		Handler: mux,
	}

	err := s.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Errorf("extension api proxy: unexpected error while serving the proxy: %v", err)
	}
}

func (rp *runtimeProxy) handle(w http.ResponseWriter, r *http.Request) {
	log.Debug("runtime api proxy: processing request")
	rp.proxy.Transport = &proxyTransport{
		processor: rp.processor,
	}
	rp.proxy.ServeHTTP(w, r)
}

func newProxy(target string, processor invocationlifecycle.InvocationProcessor) *runtimeProxy {
	url := &url.URL{
		Scheme: "http",
		Host:   target,
	}
	return &runtimeProxy{
		target:    url,
		proxy:     httputil.NewSingleHostReverseProxy(url),
		processor: processor,
	}
}
