// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"errors"
	"net/http"

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
	mux := http.NewServeMux()
	profilesHandler := e.traceAgent.GetHTTPHandler("/profiling/v1/input")
	if profilesHandler == nil {
		return errors.New("cannot create HTTP server: profiling handler is nil")
	}
	mux.Handle("/profiling/v1/input", profilesHandler)

	e.server = &http.Server{
		Addr:    "localhost:" + e.endpoint(),
		Handler: mux,
	}
	return nil
}

func (e *ddExtension) startServer(host component.Host) {
	e.log.Info("Starting DD Profiling Extension HTTP server at: " + "localhost:" + e.endpoint())
	if err := e.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		componentstatus.ReportStatus(host, componentstatus.NewFatalErrorEvent(err))
		e.log.Error("Unable to start ddprofiling extension server: ", err)
	}
}
