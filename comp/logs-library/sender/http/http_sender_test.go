// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

package http

import (
	"testing"

	"github.com/stretchr/testify/assert"

	secretsnoopimpl "github.com/DataDog/datadog-agent/comp/core/secrets/noop-impl"
	"github.com/DataDog/datadog-agent/comp/logs-library/client"
	"github.com/DataDog/datadog-agent/comp/logs-library/client/http"
	"github.com/DataDog/datadog-agent/comp/logs-library/metrics"
	"github.com/DataDog/datadog-agent/comp/logs-library/sender"
	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	configmock "github.com/DataDog/datadog-agent/pkg/config/mock"
)

func TestHttpDestinationFactory(t *testing.T) {
	tests := []struct {
		name               string
		endpoints          []config.Endpoint
		serverless         bool
		expectedReliable   int
		expectedUnreliable int
	}{
		{
			name: "standard configuration with multiple endpoints",
			endpoints: []config.Endpoint{
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8080",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8081",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8082",
					"is_reliable": false,
				}),
			},
			serverless:         false,
			expectedReliable:   2,
			expectedUnreliable: 1,
		},
		{
			name: "single endpoint configuration",
			endpoints: []config.Endpoint{
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8080",
					"is_reliable": true,
				}),
			},
			serverless:         false,
			expectedReliable:   1,
			expectedUnreliable: 0,
		},
		{
			name:               "empty endpoints",
			endpoints:          []config.Endpoint{},
			serverless:         false,
			expectedReliable:   0,
			expectedUnreliable: 0,
		},
		{
			name: "serverless configuration",
			endpoints: []config.Endpoint{
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8080",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:8081",
					"is_reliable": false,
				}),
			},
			serverless:         true,
			expectedReliable:   1,
			expectedUnreliable: 1,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Setup
			endpoints := config.NewMockEndpoints(tc.endpoints)
			destinationsCtx := client.NewDestinationsContext()
			pipelineMonitor := metrics.NewNoopPipelineMonitor("test")
			mockConfig := configmock.New(t)

			factory := httpDestinationFactory(
				endpoints,
				destinationsCtx,
				pipelineMonitor,
				sender.NewMockServerlessMeta(tc.serverless),
				mockConfig,
				"test-component",
				"application/json",
				"",
				1,
				10,
				secretsnoopimpl.NewComponent().Comp,
			)

			// Test 1: Verify first call creates destinations
			destinations1 := factory("test")
			assert.NotNil(t, destinations1)

			// Verify destination quantities
			reliable := destinations1.Reliable
			unreliable := destinations1.Unreliable

			assert.Equal(t, tc.expectedReliable, len(reliable),
				"Expected %d reliable destinations, got %d",
				tc.expectedReliable, len(reliable))
			assert.Equal(t, tc.expectedUnreliable, len(unreliable),
				"Expected %d unreliable destinations, got %d",
				tc.expectedUnreliable, len(unreliable))

			// Verify all destinations are of correct type based on serverless flag
			for _, dest := range reliable {
				if tc.serverless {
					assert.IsType(t, &http.SyncDestination{}, dest,
						"Expected reliable destination to be of type *http.SyncDestination in serverless mode")
				} else {
					assert.IsType(t, &http.Destination{}, dest,
						"Expected reliable destination to be of type *http.Destination in normal mode")
				}
			}
			for _, dest := range unreliable {
				if tc.serverless {
					assert.IsType(t, &http.SyncDestination{}, dest,
						"Expected unreliable destination to be of type *http.SyncDestination in serverless mode")
				} else {
					assert.IsType(t, &http.Destination{}, dest,
						"Expected unreliable destination to be of type *http.Destination in normal mode")
				}
			}

			// Test 2: Verify second call creates new destination instances
			destinations2 := factory("test")
			assert.NotNil(t, destinations2)
			assert.NotSame(t, destinations1, destinations2,
				"Factory should create new destinations instance")
		})
	}
}

// TestNewHTTPSender_UsesCallerPipelineMonitor is the regression for the backpressure
// status flip-flop. The component utilization/capacity snapshots live in a process-global
// registry keyed by component name:instance (e.g. "worker:q0s0",
// "destination_reliable_0:q0s0"). Those keys are only unique within a single pipeline
// monitor, so every sender that owns a real TelemetryPipelineMonitor collides on them. The
// event-platform forwarder spins up ~19 passthrough senders alongside the one logs sender;
// when NewHTTPSender fabricated its own real monitor, all 20 wrote the same keys and the
// logs status page flapped between the saturated logs sender and the idle passthrough
// senders every second. The fix threads the caller's monitor through so passthrough
// pipelines can pass a NoopPipelineMonitor and stay out of the registry. This test pins
// that contract: NewHTTPSender must use the monitor it is given, not create one.
func TestNewHTTPSender_UsesCallerPipelineMonitor(t *testing.T) {
	endpoints := config.NewMockEndpoints([]config.Endpoint{
		config.NewMockEndpointWithOptions(map[string]interface{}{
			"host":        "localhost:8080",
			"is_reliable": true,
		}),
	})
	mockConfig := configmock.New(t)
	noop := metrics.NewNoopPipelineMonitor("test")

	s := NewHTTPSender(
		mockConfig,
		&sender.NoopSink{},
		1,
		sender.NewServerlessMeta(false),
		endpoints,
		client.NewDestinationsContext(),
		"test-component",
		"application/json",
		"",
		1,
		1,
		1,
		1,
		secretsnoopimpl.NewComponent().Comp,
		noop,
	)

	assert.Same(t, noop, s.PipelineMonitor(),
		"NewHTTPSender must use the caller-provided pipeline monitor, not fabricate its own — "+
			"a passthrough pipeline passing a Noop monitor must not register global snapshots")
}
