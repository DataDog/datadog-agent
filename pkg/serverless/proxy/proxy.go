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

	"github.com/DataDog/datadog-agent/pkg/serverless/daemon"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type runtimeProxy struct {
	target                   *url.URL
	proxy                    *httputil.ReverseProxy
	currentInvocationDetails *invocationDetails
	processor                invocationProcessor
}

// Start starts the proxy
// This proxy allows us to inspect traffic from/to the AWS Lambda Runtime API
func Start(daemon *daemon.Daemon, proxyHostPort string, originalRuntimeHostPort string) bool {
	if strings.ToLower(os.Getenv("DD_EXPERIMENTAL_ENABLE_PROXY")) == "true" {
		log.Debug("the experimental proxy feature is enabled")
		go setup(proxyHostPort, originalRuntimeHostPort, &proxyProcessor{
			outChanel: daemon.TraceAgent.Get().In,
		})
		return true
	}
	return false
}

func setup(proxyHostPort string, originalRuntimeHostPort string, processor invocationProcessor) {
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
		currentInvocationDetails: rp.currentInvocationDetails,
		processor:                rp.processor,
	}
	rp.proxy.ServeHTTP(w, r)
}

func startProxy(target string, processor invocationProcessor) *runtimeProxy {
	url := &url.URL{
		Scheme: "http",
		Host:   target,
	}
	return &runtimeProxy{
		target:                   url,
		proxy:                    httputil.NewSingleHostReverseProxy(url),
		currentInvocationDetails: &invocationDetails{},
		processor:                processor,
	}
}
