// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"errors"
	"fmt"
	"net/http"
	"os"

	"go.opentelemetry.io/collector/component"
	"go.opentelemetry.io/collector/component/componentstatus"
)

func (e *ddExtension) endpoint() (endpoint string) {
	if e.cfg.Endpoint != "" {
		endpoint = e.cfg.Endpoint
	} else {
		endpoint = defaultEndpoint
	}
	return
}

func (e *ddExtension) newServer() error {
	fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] newServer: calling traceAgent.GetHTTPHandler(/profiling/v1/input), traceAgent_nil=%t\n", e.traceAgent == nil)
	mux := http.NewServeMux()
	profilesHandler := e.traceAgent.GetHTTPHandler("/profiling/v1/input")
	fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] newServer: GetHTTPHandler returned nil=%t\n", profilesHandler == nil)
	if profilesHandler == nil {
		return errors.New("cannot create HTTP server: profiling handler is nil")
	}
	// Wrap the handler so we can observe every request that reaches the server.
	wrapped := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] :7501 server received request: %s %s from %s\n", r.Method, r.URL.Path, r.RemoteAddr)
		profilesHandler.ServeHTTP(w, r)
	})
	mux.Handle("/profiling/v1/input", wrapped)

	e.server = &http.Server{
		Addr:    "localhost:" + e.endpoint(),
		Handler: mux,
	}
	fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] newServer: server configured at localhost:%s\n", e.endpoint())
	return nil
}

func (e *ddExtension) startServer(host component.Host) {
	fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] startServer: ListenAndServe on localhost:%s\n", e.endpoint())
	e.log.Info("Starting DD Profiling Extension HTTP server at: " + "localhost:" + e.endpoint())
	if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fmt.Fprintf(os.Stderr, "[DDPROF-DEBUG] startServer: ListenAndServe error: %v\n", err)
		componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		e.log.Error("Unable to start ddprofiling extension server: ", err)
	}
}
