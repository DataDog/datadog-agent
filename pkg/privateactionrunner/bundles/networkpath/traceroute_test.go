// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_networkpath

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type mockTraceroute struct {
	mock.Mock
}

func (m *mockTraceroute) Run(ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
	args := m.Called(ctx, cfg)
	return args.Get(0).(payload.NetworkPath), args.Error(1)
}

func TestNewNetworkPath(t *testing.T) {
	mockTr := &mockTraceroute{}
	bundle := NewNetworkPath(mockTr)

	assert.NotNil(t, bundle)
	assert.NotNil(t, bundle.GetAction("traceroute"))
	assert.Nil(t, bundle.GetAction("nonexistent"))
}

func TestTracerouteHandler_Run(t *testing.T) {
	tests := []struct {
		name           string
		inputs         map[string]interface{}
		mockPath       payload.NetworkPath
		mockError      error
		expectedError  string
		validateOutput func(t *testing.T, output interface{})
	}{
		{
			name: "successful traceroute",
			inputs: map[string]interface{}{
				"hostname":           "example.com",
				"port":               float64(443),
				"sourceService":      "my-service",
				"destinationService": "target-service",
				"maxTTL":             float64(30),
				"protocol":           "TCP",
				"tcpMethod":          "syn",
				"timeout":            float64(time.Second * 5),
				"tracerouteQueries":  float64(3),
				"e2eQueries":         float64(2),
				"namespace":          "test-namespace",
			},
			mockPath: payload.NetworkPath{
				Timestamp: time.Now().UnixMilli(),
				Source: payload.NetworkPathSource{
					Hostname: "source-host",
				},
				Destination: payload.NetworkPathDestination{
					Hostname: "example.com",
					Port:     443,
				},
				Protocol: payload.ProtocolTCP,
				Traceroute: payload.Traceroute{
					Runs: []payload.TracerouteRun{
						{
							RunID: "run-1",
							Hops: []payload.TracerouteHop{
								{
									TTL:       1,
									IPAddress: net.ParseIP("192.168.1.1"),
									RTT:       1.5,
									Reachable: true,
								},
							},
						},
					},
				},
			},
			mockError: nil,
			validateOutput: func(t *testing.T, output interface{}) {
				out, ok := output.(*TracerouteOutputs)
				require.True(t, ok)
				assert.Equal(t, "test-namespace", out.Path.Namespace)
				assert.Equal(t, "my-service", out.Path.Source.Service)
				assert.Equal(t, "target-service", out.Path.Destination.Service)
			},
		},
		{
			name: "traceroute execution error",
			inputs: map[string]interface{}{
				"hostname":           "example.com",
				"port":               float64(443),
				"sourceService":      "my-service",
				"destinationService": "target-service",
				"maxTTL":             float64(30),
				"protocol":           "TCP",
				"tcpMethod":          "syn",
				"timeout":            float64(time.Second * 5),
				"tracerouteQueries":  float64(3),
				"e2eQueries":         float64(2),
				"namespace":          "test-namespace",
			},
			mockPath:      payload.NetworkPath{},
			mockError:     errors.New("network error"),
			expectedError: "failed to trace path: network error",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			mockTr := &mockTraceroute{}
			handler := NewTracerouteHandler(mockTr)

			task := &types.Task{
				Data: struct {
					ID         string            `json:"id,omitempty"`
					Type       string            `json:"type,omitempty"`
					Attributes *types.Attributes `json:"attributes,omitempty"`
				}{
					Attributes: &types.Attributes{
						Inputs: tc.inputs,
					},
				},
			}

			mockTr.On("Run", mock.Anything, mock.AnythingOfType("config.Config")).Return(tc.mockPath, tc.mockError)

			output, err := handler.Run(context.Background(), task, nil)

			if tc.expectedError != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tc.expectedError)
				return
			}

			require.NoError(t, err)
			if tc.validateOutput != nil {
				tc.validateOutput(t, output)
			}

			mockTr.AssertExpectations(t)
		})
	}
}

func TestTracerouteInputs_ConfigMapping(t *testing.T) {
	mockTr := &mockTraceroute{}
	handler := NewTracerouteHandler(mockTr)

	inputs := map[string]interface{}{
		"hostname":           "example.com",
		"port":               float64(8080),
		"sourceService":      "source-svc",
		"destinationService": "dest-svc",
		"maxTTL":             float64(64),
		"protocol":           "UDP",
		"tcpMethod":          "sack",
		"timeout":            float64(time.Second * 10),
		"tracerouteQueries":  float64(5),
		"e2eQueries":         float64(4),
		"namespace":          "custom-ns",
	}

	task := &types.Task{
		Data: struct {
			ID         string            `json:"id,omitempty"`
			Type       string            `json:"type,omitempty"`
			Attributes *types.Attributes `json:"attributes,omitempty"`
		}{
			Attributes: &types.Attributes{
				Inputs: inputs,
			},
		},
	}

	var capturedCfg config.Config
	mockTr.On("Run", mock.Anything, mock.AnythingOfType("config.Config")).
		Run(func(args mock.Arguments) {
			capturedCfg = args.Get(1).(config.Config)
		}).
		Return(payload.NetworkPath{
			Timestamp: time.Now().UnixMilli(),
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{},
			},
		}, nil)

	_, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	assert.Equal(t, "example.com", capturedCfg.DestHostname)
	assert.Equal(t, uint16(8080), capturedCfg.DestPort)
	assert.Equal(t, "source-svc", capturedCfg.SourceService)
	assert.Equal(t, "dest-svc", capturedCfg.DestinationService)
	assert.Equal(t, uint8(64), capturedCfg.MaxTTL)
	assert.Equal(t, payload.Protocol("UDP"), capturedCfg.Protocol)
	assert.Equal(t, payload.TCPMethod("sack"), capturedCfg.TCPMethod)
	assert.Equal(t, time.Duration(time.Second*10), capturedCfg.Timeout)
	assert.Equal(t, 5, capturedCfg.TracerouteQueries)
	assert.Equal(t, 4, capturedCfg.E2eQueries)
	assert.True(t, capturedCfg.ReverseDNS)
}
