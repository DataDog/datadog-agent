// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package proxy

import (
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"strings"

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
func Start(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) bool {
	if strings.ToLower(os.Getenv("DD_EXPERIMENTAL_ENABLE_PROXY")) == "true" {
		log.Debug("the experimental proxy feature is enabled")
		go setup(proxyHostPort, originalRuntimeHostPort, processor)
		return true
	}
	return false
}

func setup(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) {
	proxy := startProxy(originalRuntimeHostPort, processor)

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.handle)

	s := &http.Server{
		Addr:    proxyHostPort,
		Handler: mux,
	}

	err := s.ListenAndServe()
	if err != nil && err != http.ErrServerClosed {
		log.Errorf("[proxy] error while serving the proxy")
	}

}

func (rp *runtimeProxy) handle(w http.ResponseWriter, r *http.Request) {
	rp.proxy.Transport = &proxyTransport{
		processor: rp.processor,
	}
	rp.proxy.ServeHTTP(w, r)
}

func startProxy(target string, processor invocationlifecycle.InvocationProcessor) *runtimeProxy {
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
