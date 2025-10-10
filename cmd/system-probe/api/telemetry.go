// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	pbcore "github.com/DataDog/datadog-agent/pkg/proto/pbgo/core"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// SystemProbeTelemetryProvider implements the TelemetryProvider gRPC service for system-probe
type SystemProbeTelemetryProvider struct {
	telemetryComp telemetry.Component
	pbcore.UnimplementedTelemetryProviderServer
}

// NewSystemProbeTelemetryProvider creates a new telemetry provider for system-probe
func NewSystemProbeTelemetryProvider(telemetryComp telemetry.Component) *SystemProbeTelemetryProvider {
	return &SystemProbeTelemetryProvider{
		telemetryComp: telemetryComp,
	}
}

// GetTelemetry implements the TelemetryProvider.GetTelemetry gRPC method
func (s *SystemProbeTelemetryProvider) GetTelemetry(ctx context.Context, req *pbcore.GetTelemetryRequest) (*pbcore.GetTelemetryResponse, error) {
	log.Debugf("Received telemetry request: %v", req)

	// Get telemetry from the existing telemetry component
	handler := s.telemetryComp.Handler()

	// Create a fake HTTP request to get the telemetry data
	fakeReq, err := http.NewRequestWithContext(ctx, "GET", "/telemetry", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create telemetry request: %w", err)
	}

	// Create a response recorder to capture the telemetry output
	recorder := &responseRecorder{}
	handler.ServeHTTP(recorder, fakeReq)

	// HTTP handlers that don't call WriteHeader implicitly return 200
	if recorder.code == 0 {
		recorder.code = http.StatusOK
	}

	if recorder.code != http.StatusOK {
		return nil, fmt.Errorf("telemetry handler returned status %d", recorder.code)
	}

	// Filter to only COAT metrics - avoid sending duplicates
	coatMetrics := filterCOATMetrics(recorder.body)

	return &pbcore.GetTelemetryResponse{
		Payload: &pbcore.GetTelemetryResponse_PromText{
			PromText: coatMetrics,
		},
	}, nil
}

// responseRecorder is a simple HTTP response recorder
type responseRecorder struct {
	code int
	body string
}

func (r *responseRecorder) Header() http.Header {
	return make(http.Header)
}

func (r *responseRecorder) Write(data []byte) (int, error) {
	r.body += string(data)
	return len(data), nil
}

func (r *responseRecorder) WriteHeader(statusCode int) {
	r.code = statusCode
}
