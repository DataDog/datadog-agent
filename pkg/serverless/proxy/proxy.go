// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(SERV) Fix revive linter
package proxy

import (
	"context"
	"errors"
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

// Start starts the proxy, and returns a function that can be used to shut it down cleanly.
// This proxy allows us to inspect traffic from/to the AWS Lambda Runtime API
func Start(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor) func(context.Context) error {
	stop := make(chan func(context.Context) error, 1)
	go setup(proxyHostPort, originalRuntimeHostPort, processor, stop)
	return <-stop
}

// setup creates a new http.Server and starts listening on the given proxyHostPort using the provided processor. The
// shutdownChan will be sent the shutdown function to call in order to attempt to cleanly stop the http.Server. If nil,
// the server will be impossible to cleanly shut down.
func setup(proxyHostPort string, originalRuntimeHostPort string, processor invocationlifecycle.InvocationProcessor, shutdownChan chan<- func(context.Context) error) {
	log.Debugf("runtime api proxy: starting reverse proxy on %s and forwarding to %s", proxyHostPort, originalRuntimeHostPort)
	proxy := newProxy(originalRuntimeHostPort, processor)

	mux := http.NewServeMux()
	mux.HandleFunc("/", proxy.handle)

	s := &http.Server{
		Addr:    proxyHostPort,
		Handler: mux,
	}
	if shutdownChan != nil {
		shutdownChan <- s.Shutdown
		close(shutdownChan)
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
	proxy := httputil.NewSingleHostReverseProxy(url)

	// The default error handler logs "http: proxy error: %v" then returns an HTTP 502 (bad gateway)
	// response. This is unfortunate because it lacks much any context on the original request that
	// failed, and the commonly observed error today is "context deadline exceeded", which is not
	// actionnable if you don't know what request it was for. It also logs to STDERR and does not
	// honor the agent's log level.
	proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, err error) {
		log.Debugf(
			"[serverless/proxy][%T] %s %s -- proxy error: %v",
			// The dynamic type of processor informs about what kind of proxy this was (main/appsec)
			processor,
			// The request method and URL are useful to understand what exactly failed. We won't log
			// the body (too large) or headers (risks containing sensitive data, such as API keys)
			r.Method, r.URL,
			// What happened that caused us to be called?
			err,
		)

		// If the error is a [context.DeadlineExceeded], we return an HTTP 504 (gateway timeout)
		// instead of the generic HTTP 502 (bad gateway) to give the client a better idea of what is
		// going on (this may influence retry behavior, for example).
		if errors.Is(err, context.DeadlineExceeded) {
			w.WriteHeader(http.StatusGatewayTimeout)
		} else {
			// Return an HTTP 502 (bad gateway) error response; defer the retrying to the client.
			w.WriteHeader(http.StatusBadGateway)
		}

		// Writing the error message as best-effort, we simply debug-log any error that occur here.
		if _, err := w.Write([]byte(err.Error())); err != nil {
			log.Debugf("[serverless/proxy][%T] failed to write error message to response body: %v", processor, err)
		}
	}

	return &runtimeProxy{
		target:    url,
		proxy:     proxy,
		processor: processor,
	}
}
