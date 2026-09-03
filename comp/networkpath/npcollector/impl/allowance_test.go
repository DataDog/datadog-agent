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
	"time"

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
	now := MockTimeNow()
	_, collector := newTestNpCollector(t, map[string]any{
		"network_path.connections_monitoring.enabled": true,
		"network_path.collector.filters":              []map[string]any{},
	}, &teststatsd.Client{}, &tracerouteRunner{func(_ctx context.Context, cfg config.Config) (payload.NetworkPath, error) {
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
	}})
	collector.TimeNowFn = func() time.Time { return now }

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

	run := func(profile payload.DynamicTestProfile) {
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

	for i := 0; i < standardAllowancePerHour+2; i++ {
		run(payload.DynamicTestProfileStandard)
	}
	require.Len(t, emitted, standardAllowancePerHour+2)
	for i, path := range emitted {
		assert.Equal(t, i < standardAllowancePerHour, path.InAllowance)
	}

	now = now.Add(time.Hour)
	emitted = nil
	run(payload.DynamicTestProfileStandard)
	run(payload.DynamicTestProfileBasic)
	require.Len(t, emitted, 2)
	assert.True(t, emitted[0].InAllowance)
	assert.True(t, emitted[1].InAllowance)
}
