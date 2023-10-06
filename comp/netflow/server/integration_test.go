// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package server

import (
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/fx"

	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"

	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"

	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
)

func singleListenerConfig(flowType common.FlowType, port uint16) *nfconfig.NetflowConfig {
	return &nfconfig.NetflowConfig{
		Enabled:                 true,
		AggregatorFlushInterval: 1,
		Listeners: []nfconfig.ListenerConfig{{
			FlowType: flowType,
			BindHost: "127.0.0.1",
			Port:     port,
		}},
	}
}

var flushTime, _ = time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

var setTimeNow = fx.Invoke(func(c Component) {
	c.(*Server).FlowAgg.TimeNowFunction = func() time.Time {
		return flushTime
	}
})

func assertFlowEventsCount(t *testing.T, port uint16, srv *Server, packetData []byte, expectedEvents uint64) bool {
	return assert.EventuallyWithT(t, func(c *assert.CollectT) {
		err := testutil.SendUDPPacket(port, packetData)
		assert.NoError(c, err, "error sending udp packet")
		if err != nil {
			return
		}

		netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 1*time.Second, 2)
		assert.Equal(c, expectedEvents, netflowEvents)
		assert.NoError(c, err)
	}, 10*time.Second, 10*time.Millisecond)
}

func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("netflow5", port),
		),
		setTimeNow,
	)).(*Server)

	// Set expectations
	testutil.ExpectNetflow5Payloads(t, epForwarder)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	packetData, err := testutil.GetNetFlow5Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 2)
}

func TestNetFlow_IntegrationTest_NetFlow9(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("netflow9", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")

	assertFlowEventsCount(t, port, srv, packetData, 29)
}

func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	var epForwarder forwarder.MockComponent
	srv := fxutil.Test[Component](t, fx.Options(
		testOptions,
		fx.Populate(&epForwarder),
		fx.Replace(
			singleListenerConfig("sflow5", port),
		),
		setTimeNow,
	)).(*Server)

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	packetData, err := testutil.GetSFlow5Packet()
	require.NoError(t, err, "error getting sflow data")

	assertFlowEventsCount(t, port, srv, packetData, 7)
}
