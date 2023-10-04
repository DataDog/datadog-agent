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

	"github.com/DataDog/datadog-agent/comp/ndmtmp/forwarder"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	nfconfig "github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
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

//
//func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
//	port := testutil.GetFreePort()
//	var epForwarder forwarder.MockComponent
//	srv := fxutil.Test[Component](t, fx.Options(
//		testOptions,
//		fx.Populate(&epForwarder),
//		fx.Replace(
//			singleListenerConfig("netflow5", port),
//		),
//		setTimeNow,
//	)).(*Server)
//
//	// Set expectations
//	testutil.ExpectNetflow5Payloads(t, epForwarder)
//	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)
//
//	// Send netflowV5Data twice to test aggregator
//	// Flows will have 2x bytes/packets after aggregation
//	packetData, err := testutil.GetNetFlow5Packet()
//	require.NoError(t, err, "error getting packet")
//	err = testutil.SendUDPPacket(port, packetData)
//	require.NoError(t, err, "error sending udp packet")
//
//	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 2)
//	assert.Equal(t, uint64(2), netflowEvents)
//	assert.NoError(t, err)
//}

func TestNetFlow_IntegrationTest_NetFlow9_1(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_2(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_3(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_4(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_5(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_6(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_7(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_8(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_9(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_10(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_11(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_12(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_13(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_14(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_15(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_16(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_17(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_18(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_19(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_20(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_21(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_22(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_23(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_24(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_25(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_26(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_27(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_28(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_29(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_30(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_31(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_32(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_33(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_34(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_35(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_36(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_37(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_38(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_39(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_40(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_41(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_42(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_43(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_44(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_45(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_46(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_47(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_48(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_49(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_50(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_51(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_52(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_53(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_54(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_55(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_56(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_57(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_58(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_59(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_60(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_61(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_62(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_63(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_64(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_65(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_66(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_67(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_68(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_69(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_70(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_71(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_72(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_73(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_74(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_75(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_76(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_77(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_78(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_79(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_80(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_81(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_82(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_83(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_84(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_85(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_86(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_87(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_88(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_89(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_90(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_91(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_92(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_93(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_94(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_95(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_96(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_97(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_98(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_99(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_100(t *testing.T) {
	port := testutil.GetFreePort()
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
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

//
//func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
//	port := testutil.GetFreePort()
//	var epForwarder forwarder.MockComponent
//	srv := fxutil.Test[Component](t, fx.Options(
//		testOptions,
//		fx.Populate(&epForwarder),
//		fx.Replace(
//			singleListenerConfig("sflow5", port),
//		),
//		setTimeNow,
//	)).(*Server)
//
//	// Test later content of payloads if needed for more precise test.
//	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
//	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)
//
//	data, err := testutil.GetSFlow5Packet()
//	require.NoError(t, err, "error getting sflow data")
//
//	err = testutil.SendUDPPacket(port, data)
//	require.NoError(t, err, "error sending udp packet")
//
//	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, 30*time.Second, 6)
//	assert.Equal(t, uint64(7), netflowEvents)
//	assert.NoError(t, err)
//}
