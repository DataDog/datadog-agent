// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package com_datadoghq_ddagent_networkpath

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	tracerouteconfig "github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/privateactionrunner/types"
	"github.com/DataDog/datadog-agent/pkg/util/option"
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

type mockEventPlatformForwarder struct {
	lastEventType string
	lastMessage   *message.Message
	sendCount     int
	err           error
}

func (m *mockEventPlatformForwarder) SendEventPlatformEvent(msg *message.Message, eventType string) error {
	m.lastMessage = msg
	m.lastEventType = eventType
	m.sendCount++
	return m.err
}

func (m *mockEventPlatformForwarder) SendEventPlatformEventBlocking(msg *message.Message, eventType string) error {
	return m.SendEventPlatformEvent(msg, eventType)
}

func (m *mockEventPlatformForwarder) Purge() map[string][]*message.Message {
	return nil
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
	handler := NewGetNetworkPathHandler(tracerouteStub, option.NonePtr[eventplatform.Forwarder]())

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
		},
	}

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)

	result, ok := output.(*payload.NetworkPath)
	require.True(t, ok)
	require.Equal(t, "default", result.Namespace)
	require.Equal(t, payload.PathOriginNetworkPathIntegration, result.Origin)
	require.Equal(t, payload.TestRunTypeTriggered, result.TestRunType)
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
	handler := NewGetNetworkPathHandler(tracerouteStub, option.NonePtr[eventplatform.Forwarder]())

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
	handler := NewGetNetworkPathHandler(tracerouteStub, option.NonePtr[eventplatform.Forwarder]())

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

func TestGetNetworkPathHandlerRunSendToBackend(t *testing.T) {
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
						Destination: payload.TracerouteDestination{
							IPAddress: net.ParseIP("1.2.3.4"),
						},
					},
				},
			},
		},
	}
	forwarder := &mockEventPlatformForwarder{}
	eventPlatform := option.NewPtr[eventplatform.Forwarder](forwarder)
	handler := NewGetNetworkPathHandler(tracerouteStub, eventPlatform)

	task := &types.Task{}
	task.Data.Attributes = &types.Attributes{
		Inputs: map[string]interface{}{
			"hostname":      "example.com",
			"port":          uint16(443),
			"namespace":     "default",
			"sendToBackend": true,
		},
	}

	output, err := handler.Run(context.Background(), task, nil)
	require.NoError(t, err)
	require.NotNil(t, output)
	require.Equal(t, 1, forwarder.sendCount)
	require.Equal(t, eventplatform.EventTypeNetworkPath, forwarder.lastEventType)

	var sent payload.NetworkPath
	require.NoError(t, json.Unmarshal(forwarder.lastMessage.GetContent(), &sent))
	require.Equal(t, "default", sent.Namespace)
	require.Equal(t, payload.PathOriginNetworkPathIntegration, sent.Origin)
	require.Equal(t, payload.TestRunTypeTriggered, sent.TestRunType)
	require.Equal(t, payload.SourceProductNetworkPath, sent.SourceProduct)
	require.Equal(t, payload.CollectorTypeAgent, sent.CollectorType)
}
