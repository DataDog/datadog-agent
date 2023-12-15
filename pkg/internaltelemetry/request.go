// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2023-present Datadog, Inc.

// Package internaltelemetry full description in README.md
package internaltelemetry

import (
	"net/http"
)

func (lts *logTelemetrySender) addTelemetryHeaders(req *http.Request, apikey, bodylen string) {

	req.Header.Add("DD-Api-Key", apikey)
	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Content-Length", bodylen)
	req.Header.Add("DD-Telemetry-api-version", "v2")
	req.Header.Add("DD-Telemetry-request-type", "v2") // todo
	req.Header.Add("dd-client-library-language", "agent")
	req.Header.Add("dd-client-library-version", "1.5") // should this be agent version?
	req.Header.Add("datadog-container-id", "1")        // todo is this necessary?  likely not a container
}
