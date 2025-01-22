// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// Package ddprofilingextensionimpl defines the OpenTelemetry Profiling implementation
package ddprofilingextensionimpl

import (
	"log"
	"net/http"
)

func (e *ddExtension) StartServer() {
	http.Handle("/profiling/v1/input", e.traceAgent.GetHTTPHandler("/profiling/v1/input"))

	var endpoint string
	if e.cfg.Endpoint != "" {
		endpoint = e.cfg.Endpoint
	} else {
		endpoint = defaultEndpoint
	}
	log.Fatal(http.ListenAndServe("localhost:"+endpoint, nil))
}
