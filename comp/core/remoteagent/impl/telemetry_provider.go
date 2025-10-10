// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package impl

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/flavor"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// telemetryProvider implements the TelemetryProvider gRPC service
type telemetryProvider struct {
	telemetryComp telemetry.Component
	pbcore.UnimplementedTelemetryProviderServer
}

// newTelemetryProvider creates a new telemetry provider
func newTelemetryProvider(telemetryComp telemetry.Component) *telemetryProvider {
	return &telemetryProvider{
		telemetryComp: telemetryComp,
	}
}

// GetTelemetry implements the TelemetryProvider.GetTelemetry gRPC method
func (t *telemetryProvider) GetTelemetry(ctx context.Context, req *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	log.Debugf("Received telemetry request: %v", req)

	// Get telemetry from the existing telemetry component
	handler := t.telemetryComp.Handler()

	// Create a fake HTTP request to get the telemetry data
	fakeReq, err := http.NewRequestWithContext(ctx, "GET", "/telemetry", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create telemetry request: %w", err)
	}

	// Create a response recorder to capture the telemetry output
	recorder := &httpResponseRecorder{}
	handler.ServeHTTP(recorder, fakeReq)

	// HTTP handlers that don't call WriteHeader implicitly return 200
	if recorder.code == 0 {
		recorder.code = http.StatusOK
	}

	if recorder.code != http.StatusOK {
		return nil, fmt.Errorf("telemetry handler returned status %d", recorder.code)
	}

	// Filter to only COAT metrics for the current agent type
	coatMetrics := filterCOATMetrics(recorder.body)

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: coatMetrics,
		},
	}, nil
}

// httpResponseRecorder is a simple HTTP response recorder
type httpResponseRecorder struct {
	code int
	body string
}

func (r *httpResponseRecorder) Header() http.Header {
	return make(http.Header)
}

func (r *httpResponseRecorder) Write(data []byte) (int, error) {
	r.body += string(data)
	return len(data), nil
}

func (r *httpResponseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}

// filterCOATMetrics extracts only COAT metrics for the current agent type from Prometheus text format
func filterCOATMetrics(promText string) string {
	lines := strings.Split(promText, "\n")
	var coatLines []string

	// Get the current agent flavor to determine which metrics to filter
	agentFlavor := flavor.GetFlavor()
	var metricPrefix string

	switch agentFlavor {
	case flavor.SystemProbe:
		metricPrefix = "system_probe_"
	case flavor.TraceAgent:
		metricPrefix = "trace_agent_"
	// Add more cases as needed for other agents
	default:
		// For unknown agents, don't filter anything
		return promText
	}

	inCOATMetric := false
	for _, line := range lines {
		// Check if this line starts a COAT metric (# HELP or # TYPE <prefix>*)
		helpPrefix := "# HELP " + metricPrefix
		typePrefix := "# TYPE " + metricPrefix

		if strings.HasPrefix(line, helpPrefix) || strings.HasPrefix(line, typePrefix) {
			inCOATMetric = true
			coatLines = append(coatLines, line)
		} else if strings.HasPrefix(line, metricPrefix) {
			// Metric value line for COAT metric
			coatLines = append(coatLines, line)
			inCOATMetric = false
		} else if strings.HasPrefix(line, "# ") {
			// Different metric's help/type - no longer in COAT metric
			inCOATMetric = false
		} else if inCOATMetric {
			// Continuation line for COAT metric (multiline help text)
			coatLines = append(coatLines, line)
		}
		// Skip all other lines (non-COAT metrics)
	}

	result := strings.Join(coatLines, "\n")
	if len(result) > 0 {
		result += "\n"
	}
	return result
}
