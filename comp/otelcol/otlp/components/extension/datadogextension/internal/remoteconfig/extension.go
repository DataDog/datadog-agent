// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package remoteconfig

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
	"go.opentelemetry.io/collector/config/confighttp"
	"go.opentelemetry.io/collector/extension"
)

type forwarder struct {
	config   *Config
	settings extension.Settings
	key      string
	backend  *url.URL
	egress   confighttp.ClientConfig
	client   *http.Client
	server   *http.Server
	listener net.Listener
	cancel   context.CancelFunc
	done     chan struct{}
}

// New owns agentless RC forwarding without an Agent RC cache or polling engine.
func New(set extension.Settings, c *Config, key, site string, egress confighttp.ClientConfig) (extension.Extension, error) {
	if err := ValidateConfig(c, site); err != nil {
		return nil, err
	}
	if strings.TrimSpace(key) == "" {
		return nil, errors.New("api.key must be configured in the Collector")
	}
	backend := &url.URL{Scheme: "https", Host: "config." + site}
	return &forwarder{config: c, settings: set, key: key, backend: backend, egress: egress}, nil
}

func (f *forwarder) Start(ctx context.Context, host component.Host) error {
	client, err := f.egress.ToClient(ctx, host.GetExtensions(), f.settings.TelemetrySettings)
	if err != nil {
		return err
	}
	client.Timeout = f.config.RequestTimeout
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	lifetime, cancel := context.WithCancel(context.Background())
	handler := http.TimeoutHandler(http.HandlerFunc(f.serveHTTP), f.config.RequestTimeout, "Remote Configuration request timed out")
	server, err := f.config.ServerConfig.ToServer(ctx, host.GetExtensions(), f.settings.TelemetrySettings, handler)
	if err != nil {
		cancel()
		client.CloseIdleConnections()
		return err
	}
	listener, err := f.config.ServerConfig.ToListener(ctx)
	if err != nil {
		cancel()
		client.CloseIdleConnections()
		return err
	}
	server.BaseContext = func(net.Listener) context.Context { return lifetime }
	f.client, f.server, f.listener, f.cancel = client, server, listener, cancel
	f.done = make(chan struct{})
	go func() {
		defer close(f.done)
		if err := server.Serve(listener); err != nil && !errors.Is(err, http.ErrServerClosed) {
			componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		}
	}()
	return nil
}

func (f *forwarder) Shutdown(ctx context.Context) error {
	if f.server == nil {
		return nil
	}
	f.cancel()
	err := f.server.Shutdown(ctx)
	if err != nil {
		_ = f.server.Close()
	}
	<-f.done
	f.client.CloseIdleConnections()
	return err
}

func (f *forwarder) serveHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
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
	if r.Method != method {
		w.Header().Set("Allow", method)
		http.Error(w, "Method Not Allowed", http.StatusMethodNotAllowed)
		return
	}
	if r.URL.RawQuery != "" || r.URL.ForceQuery || r.URL.RawPath != "" {
		http.Error(w, "query parameters and encoded paths are not supported", http.StatusBadRequest)
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, f.config.ServerConfig.MaxRequestBodySize))
	if err != nil {
		code := http.StatusBadRequest
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			code = http.StatusRequestEntityTooLarge
		}
		http.Error(w, http.StatusText(code), code)
		return
	}
	target := *f.backend
	target.Path = r.URL.Path
	request, err := http.NewRequestWithContext(r.Context(), r.Method, target.String(), bytes.NewReader(body))
	if err != nil {
		http.Error(w, "Bad Gateway", http.StatusBadGateway)
		return
	}
	headers := r.Header.Clone()
	stripHopHeaders(headers)
	request.Header = sdkHeaders(headers)
	request.Header.Set("DD-API-KEY", f.key)
	response, err := f.client.Do(request)
	if err != nil {
		code := http.StatusBadGateway
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(r.Context().Err(), context.DeadlineExceeded) {
			code = http.StatusGatewayTimeout
		}
		http.Error(w, http.StatusText(code), code)
		return
	}
	defer response.Body.Close()
	stripHopHeaders(response.Header)
	response.Header.Del("DD-API-KEY")
	for key, values := range response.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

func stripHopHeaders(headers http.Header) {
	for _, value := range headers.Values("Connection") {
		for _, key := range strings.Split(value, ",") {
			headers.Del(strings.TrimSpace(key))
		}
	}
	for _, key := range []string{"Connection", "Proxy-Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer", "Transfer-Encoding", "Upgrade"} {
		headers.Del(key)
	}
}

func sdkHeaders(in http.Header) http.Header {
	out := make(http.Header)
	for key, values := range in {
		switch strings.ToLower(key) {
		case "content-type", "content-encoding", "accept", "accept-encoding", "user-agent", "dd-client-runtime-id", "dd-client-language", "dd-client-version", "dd-evp-origin", "dd-evp-origin-version", "datadog-container-id", "datadog-entity-id", "datadog-external-env":
			out[key] = append([]string(nil), values...)
		default:
			if strings.HasPrefix(strings.ToLower(key), "datadog-meta-") {
				out[key] = append([]string(nil), values...)
			}
		}
	}
	return out
}
