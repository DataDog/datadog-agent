// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build with_telemetry

package main

import (
	"net/http"
	"os"

	"github.com/DataDog/datadog-agent/pkg/fleet/installer/telemetry"
)

// startTelemetry constructs a Telemetry sender using env-var configuration,
// so the spans the interp package registers on the global tracer actually
// get flushed to Datadog intake. Returns a stop function the caller must
// invoke before process exit to synchronously flush completed spans.
//
// Built only when the with_telemetry tag is set; the stock release binary
// uses the no-op variant in telemetry_off.go.
//
// Env vars:
//   - DD_API_KEY  (required; if unset, returns a no-op — no telemetry is sent)
//   - DD_SITE     (defaults to datadoghq.com; set to datad0g.com for staging)
//
// The HTTP client is http.DefaultClient, which respects HTTP_PROXY /
// HTTPS_PROXY / NO_PROXY env vars via Go's default transport.
func startTelemetry() func() {
	apiKey := os.Getenv("DD_API_KEY")
	if apiKey == "" {
		return func() {}
	}
	tel := telemetry.NewTelemetry(http.DefaultClient, apiKey, os.Getenv("DD_SITE"), "rshell")
	return tel.Stop
}
