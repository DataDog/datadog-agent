// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_networkpath

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteconfig "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
)

type mockTraceroute struct {
	cfg  tracerouteconfig.Config
	path payload.NetworkPath
	err  error
}

func (m *mockTraceroute) Run(_ context.Context, cfg tracerouteconfig.Config) (payload.NetworkPath, error) {
	m.cfg = cfg
	return m.path, m.err
}

func TestGetNetworkPathHandlerRun(t *testing.T) {
	tracerouteStub := &mockTraceroute{
		path: payload.NetworkPath{
			Source: payload.NetworkPathSource{Service: "old-source"},
			Destination: payload.NetworkPathDestination{
				Hostname: "example.com",
				Port:     443,
				Service:  "old-dest",
			},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{
					{
						RunID: "run-1",
						Destination: payload.TracerouteDestination{
							IPAddress: net.ParseIP("1.2.3.4"),
						},
					},
				},
			},
		},
	}
	handler := NewGetNetworkPathHandler(tracerouteStub)

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"hostname":           "example.com",
			"port":               uint16(443),
			"sourceService":      "source-service",
			"destinationService": "dest-service",
			"maxTtl":             uint8(30),
			"protocol":           payload.ProtocolTCP,
			"tcpMethod":          payload.TCPConfigSYN,
			"timeoutMs":          int64(2000),
			"tracerouteQueries":  3,
			"e2eQueries":         10,
			"namespace":          "default",
			"sendToBackend":      true,
		},
	}

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result, ok := output.(*payload.NetworkPath)
	require.True(t, ok)
	require.Equal(t, "default", result.Namespace)
	require.Equal(t, payload.PathOriginNetworkPathIntegration, result.Origin)
	require.Equal(t, payload.TestRunTypeOnDemand, result.TestRunType)
	require.Equal(t, payload.SourceProductNetworkPath, result.SourceProduct)
	require.Equal(t, payload.CollectorTypeAgent, result.CollectorType)
	require.Equal(t, "source-service", result.Source.Service)
	require.Equal(t, "dest-service", result.Destination.Service)

	expectedCfg := tracerouteconfig.Config{
		DestHostname:       "example.com",
		DestPort:           443,
		DestinationService: "dest-service",
		SourceService:      "source-service",
		MaxTTL:             30,
		Timeout:            2 * time.Second,
		Protocol:           payload.ProtocolTCP,
		TCPMethod:          payload.TCPConfigSYN,
		ReverseDNS:         true,
		TracerouteQueries:  3,
		E2eQueries:         10,
	}
	require.Equal(t, expectedCfg, tracerouteStub.cfg)
}

func TestGetNetworkPathHandlerRunDefaults(t *testing.T) {
	tracerouteStub := &mockTraceroute{
		path: payload.NetworkPath{
			Source: payload.NetworkPathSource{Service: "old-source"},
			Destination: payload.NetworkPathDestination{
				Hostname: "example.com",
				Port:     443,
				Service:  "old-dest",
			},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{
					{
						RunID: "run-1",
						Destination: payload.TracerouteDestination{
							IPAddress: net.ParseIP("1.2.3.4"),
						},
					},
				},
			},
		},
	}
	handler := NewGetNetworkPathHandler(tracerouteStub)

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"hostname": "example.com",
			"port":     uint16(443),
		},
	}

	_, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	expectedCfg := tracerouteconfig.Config{
		DestHostname:      "example.com",
		DestPort:          443,
		MaxTTL:            pkgconfigsetup.DefaultNetworkPathMaxTTL,
		Timeout:           pkgconfigsetup.DefaultNetworkPathTimeout * time.Millisecond,
		Protocol:          payload.ProtocolUDP,
		ReverseDNS:        true,
		TracerouteQueries: pkgconfigsetup.DefaultNetworkPathStaticPathTracerouteQueries,
		E2eQueries:        pkgconfigsetup.DefaultNetworkPathStaticPathE2eQueries,
	}
	require.Equal(t, expectedCfg, tracerouteStub.cfg)
}

func TestGetNetworkPathHandlerRunInvalidPath(t *testing.T) {
	tracerouteStub := &mockTraceroute{
		path: payload.NetworkPath{
			Destination: payload.NetworkPathDestination{
				Hostname: "example.com",
				Port:     443,
			},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{
					{
						RunID: "run-1",
					},
				},
			},
		},
	}
	handler := NewGetNetworkPathHandler(tracerouteStub)

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"hostname": "example.com",
			"port":     uint16(443),
		},
	}

	_, err := handler.Run(context.Background(), task, nil)
	require.ErrorContains(t, err, "invalid destination IP address")
}
