// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package datadogremoteconfigextension

import (
	"bytes"
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"time"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/extension"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
)

type remoteConfigExtension struct {
	config   *Config
	settings extension.Settings
	server   *http.Server
	client   *http.Client
	listener net.Listener
	done     chan struct{}
}

func (e *remoteConfigExtension) Start(ctx context.Context, host component.Host) error {
	upstreamTLS, err := e.config.Egress.TLS.LoadTLSConfig(ctx)
	if err != nil {
		return err
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = upstreamTLS
	transport.DisableCompression = true
	if e.config.Egress.ProxyURL != "" {
		proxyURL, _ := url.Parse(e.config.Egress.ProxyURL)
		transport.Proxy = http.ProxyURL(proxyURL)
	}
	client := &http.Client{Transport: transport}
	var tlsConfig *tls.Config
	if e.config.TLS != nil {
		tlsConfig, err = e.config.TLS.LoadTLSConfig(ctx)
		if err != nil {
			client.CloseIdleConnections()
			return err
		}
	}
	meter := e.settings.MeterProvider.Meter("github.com/DataDog/datadog-agent/comp/otelcol/otlp/components/extension/datadogremoteconfigextension")
	requests, err := meter.Int64Counter("datadog_remoteconfig.requests", metric.WithDescription("Remote Config proxy requests by route and response status."))
	if err != nil {
		client.CloseIdleConnections()
		return err
	}
	listener, err := net.Listen("tcp", e.config.Endpoint)
	if err != nil {
		client.CloseIdleConnections()
		return err
	}
	if tlsConfig != nil {
		listener = tls.NewListener(listener, tlsConfig)
	}
	e.client = client
	e.listener = listener
	e.done = make(chan struct{})
	e.server = &http.Server{
		Handler:           e.handler(client.Transport, requests),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30*time.Second + e.config.Egress.Timeout,
		IdleTimeout:       time.Minute,
	}
	go func() {
		defer close(e.done)
		if err := e.server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()
	return nil
}

func (e *remoteConfigExtension) Shutdown(ctx context.Context) error {
	if e.server == nil {
		return nil
	}
	err := e.server.Shutdown(ctx)
	if err != nil {
		_ = e.server.Close()
	}
	<-e.done
	e.client.CloseIdleConnections()
	return err
}

func (e *remoteConfigExtension) handler(transport http.RoundTripper, requests metric.Int64Counter) http.Handler {
	upstream, _ := url.Parse(e.config.Egress.Endpoint)
	upstream.Path = ""
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(upstream)
			r.Out.URL.RawPath = ""
		},
		Transport: transport,
		ErrorLog:  log.New(io.Discard, "", 0),
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			status := http.StatusBadGateway
			if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
				status = http.StatusGatewayTimeout
			}
			http.Error(w, http.StatusText(status), status)
		},
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method := ""
		switch r.URL.Path {
		case "/api/v0.1/org":
			method = http.MethodGet
		case "/api/v0.1/configurations":
			method = http.MethodPost
		default:
			http.NotFound(w, r)
			return
		}
		response := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			requests.Add(r.Context(), 1, metric.WithAttributes(
				attribute.String("http.route", r.URL.Path), attribute.Int("http.response.status_code", response.status)))
		}()
		if r.Method != method {
			response.Header().Set("Allow", method)
			http.Error(response, "Method Not Allowed", http.StatusMethodNotAllowed)
			return
		}
		if r.URL.RawQuery != "" || r.URL.ForceQuery {
			http.Error(response, "Query parameters are not supported", http.StatusBadRequest)
			return
		}
		body, err := io.ReadAll(http.MaxBytesReader(response, r.Body, e.config.MaxRequestBodySize))
		if err != nil {
			var tooLarge *http.MaxBytesError
			if errors.As(err, &tooLarge) {
				http.Error(response, "Request body too large", http.StatusRequestEntityTooLarge)
			} else {
				http.Error(response, "Failed to read request body", http.StatusBadRequest)
			}
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), e.config.Egress.Timeout)
		defer cancel()
		r = r.Clone(ctx)
		r.Body = io.NopCloser(bytes.NewReader(body))
		r.ContentLength = int64(len(body))
		r.TransferEncoding = nil
		proxy.ServeHTTP(response, r)
	})
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(status int) {
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusWriter) Unwrap() http.ResponseWriter { return w.ResponseWriter }
