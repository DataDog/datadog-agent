// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package testbenchimpl

import (
	"encoding/json"

	reporter "github.com/DataDog/datadog-agent/comp/anomalydetection/reporter/def"
)

// TestbenchReporter implements reporter.Reporter for the testbench.
// On each Report call it pushes a lightweight "advance" SSE event so connected
// browser clients know to refresh their state via the API.
// It also satisfies SSEAccess so the testbench HTTP API can subscribe clients.
type TestbenchReporter struct {
	hub *sseHub
}

// Ensure TestbenchReporter satisfies both interfaces.
var _ reporter.Reporter = (*TestbenchReporter)(nil)
var _ SSEAccess = (*TestbenchReporter)(nil)

// Requires defines dependencies for the testbench reporter component.
type Requires struct{}

// Provides defines the output of the testbench reporter component.
type Provides struct {
	Reporter  reporter.Reporter `group:"anomalydetection_reporters"`
	SSEAccess SSEAccess
}

// NewComponent is the fx constructor for the testbench reporter.
func NewComponent(_ Requires) Provides {
	r := NewTestbenchReporter()
	return Provides{Reporter: r, SSEAccess: r}
}

// NewTestbenchReporter creates a new testbench SSE reporter.
func NewTestbenchReporter() *TestbenchReporter {
	return &TestbenchReporter{hub: newSSEHub()}
}

func (r *TestbenchReporter) Name() string { return "testbench_reporter" }

func (r *TestbenchReporter) Report(output reporter.ReportOutput) bool {
	type advancePayload struct {
		AdvancedToSec int64 `json:"advancedToSec"`
		NewAnomalies  int   `json:"newAnomalies"`
		Correlations  int   `json:"correlations"`
	}
	data, _ := json.Marshal(advancePayload{
		AdvancedToSec: output.AdvancedToSec,
		NewAnomalies:  len(output.NewAnomalies),
		Correlations:  len(output.ActiveCorrelations),
	})
	r.hub.Broadcast(SSEEvent{Event: "advance", Data: data})
	return len(output.ActiveCorrelations) > 0 || len(output.NewAnomalies) > 0
}

// Subscribe registers an SSE client. Implements SSEAccess.
func (r *TestbenchReporter) Subscribe() (*SSEClient, func()) {
	return r.hub.Subscribe()
}

// LatestStatus returns the most recent status payload. Implements SSEAccess.
func (r *TestbenchReporter) LatestStatus() []byte {
	return r.hub.LatestStatus()
}

// Broadcast sends an arbitrary SSE event. Implements SSEAccess.
func (r *TestbenchReporter) Broadcast(msg SSEEvent) {
	r.hub.Broadcast(msg)
}
