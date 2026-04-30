// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.
package tcp

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/comp/logs/agent/config"
	"github.com/DataDog/datadog-agent/pkg/logs/client"
	"github.com/DataDog/datadog-agent/pkg/logs/client/tcp"
	"github.com/DataDog/datadog-agent/pkg/logs/sender"
	"github.com/DataDog/datadog-agent/pkg/logs/status/statusinterface"
)

func TestTCPDestinationFactory(t *testing.T) {
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
					"host":        "localhost:10516",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:10517",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:10518",
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
					"host":        "localhost:10516",
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
					"host":        "localhost:10516",
					"is_reliable": true,
				}),
				config.NewMockEndpointWithOptions(map[string]interface{}{
					"host":        "localhost:10517",
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
			status := statusinterface.NewStatusProviderMock()

			factory := tcpDestinationFactory(
				endpoints,
				destinationsCtx,
				sender.NewMockServerlessMeta(tc.serverless),
				status,
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

			// Verify all destinations are of type *Destination
			for _, dest := range reliable {
				assert.IsType(t, &tcp.Destination{}, dest,
					"Expected reliable destination to be of type *tcp.Destination")
			}
			for _, dest := range unreliable {
				assert.IsType(t, &tcp.Destination{}, dest,
					"Expected unreliable destination to be of type *tcp.Destination")
			}

			// Test 2: Verify second call creates new destination instances
			destinations2 := factory("test")
			assert.NotNil(t, destinations2)
			assert.NotSame(t, destinations1, destinations2,
				"Factory should create new destinations instance")
		})
	}
}
