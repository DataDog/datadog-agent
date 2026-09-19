// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

//go:build test

package npcollectorimpl

import (
	"context"
	"encoding/json"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	eventplatform "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/def"
	eventplatformimpl "github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/impl"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/common"
	"github.com/DataDog/datadog-agent/comp/networkpath/npcollector/impl/pathteststore"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/networkpath/payload"
	"github.com/DataDog/datadog-agent/pkg/networkpath/traceroute/config"
	"github.com/DataDog/datadog-agent/pkg/trace/teststatsd"
)

func TestRunTracerouteForPathAllowance(t *testing.T) {
	collector, emitted := newAllowanceCollector(t, succeedingAllowanceTraceroute())

	runAllowancePath(collector, payload.DynamicTestProfileBasic)
	runAllowancePath(collector, payload.DynamicTestProfileStandard)
	runAllowancePath(collector, "")
	runAllowanceNetflowPath(collector)

	require.Len(t, *emitted, 4)
	assert.Equal(t, payload.DynamicTestProfileBasic, (*emitted)[0].DynamicTestProfile)
	assert.Equal(t, payload.DynamicTestClassCore, (*emitted)[0].DynamicTestClass)
	assert.Equal(t, payload.DynamicTestProfileStandard, (*emitted)[1].DynamicTestProfile)
	assert.Empty(t, (*emitted)[1].DynamicTestClass)
	assert.Empty(t, (*emitted)[2].DynamicTestProfile)
	assert.Empty(t, (*emitted)[2].DynamicTestClass)
	assert.Empty(t, (*emitted)[3].DynamicTestProfile)
	assert.Empty(t, (*emitted)[3].DynamicTestClass)
}

func newAllowanceCollector(t *testing.T, traceroute *tracerouteRunner) (*npCollectorImpl, *[]payload.NetworkPath) {
	t.Helper()
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.filters":              []map[string]any{},
	}, &teststatsd.Client{}, traceroute)

	var emitted []payload.NetworkPath
	mockEpForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))
	collector.epForwarder = mockEpForwarder
	mockEpForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), eventplatform.EventTypeNetworkPath).
		DoAndReturn(func(msg *message.Message, _ string) error {
			var path payload.NetworkPath
			require.NoError(t, json.Unmarshal(msg.GetContent(), &path))
			emitted = append(emitted, path)
			return nil
		}).AnyTimes()
	return collector, &emitted
}

func succeedingAllowanceTraceroute() *tracerouteRunner {
	return &tracerouteRunner{func(_ context.Context, cfg config.Config) (payload.NetworkPath, error) {
		return payload.NetworkPath{
			Protocol: cfg.Protocol,
			Destination: payload.NetworkPathDestination{
				Hostname: cfg.DestHostname,
				Port:     cfg.DestPort,
			},
			Traceroute: payload.Traceroute{
				Runs: []payload.TracerouteRun{{
					RunID: "aa-bb-cc",
					Destination: payload.TracerouteDestination{
						IPAddress: net.ParseIP("10.0.0.2"),
						Port:      cfg.DestPort,
					},
				}},
			},
		}, nil
	}}
}

func runAllowancePath(collector *npCollectorImpl, profile payload.DynamicTestProfile) {
	collector.runTracerouteForPath(&pathteststore.PathtestContext{
		Pathtest: &common.Pathtest{
			Hostname:           "10.0.0.2",
			Port:               443,
			Protocol:           payload.ProtocolTCP,
			Origin:             payload.PathOriginNetworkTraffic,
			DynamicTestProfile: profile,
		},
	})
}

func runAllowanceNetflowPath(collector *npCollectorImpl) {
	collector.runTracerouteForPath(&pathteststore.PathtestContext{
		Pathtest: &common.Pathtest{
			Hostname: "10.0.0.2",
			Port:     443,
			Protocol: payload.ProtocolTCP,
			Origin:   payload.PathOriginNetflow,
		},
	})
}
