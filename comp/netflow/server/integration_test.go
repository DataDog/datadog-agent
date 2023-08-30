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

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	nfconfig "github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
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

func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
	port := testutil.GetFreePort()
	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	srv := fxutil.Test[Component](t, fx.Options(
		log.MockModule,
		testModule,
		fx.Replace(
			fx.Annotate(epForwarder, fx.As(new(netflowEPForwarder))),
			singleListenerConfig("netflow5", port),
		),
	)).(*Server)

	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

	// Setup NetFlow Server
	srv.FlowAgg.TimeNowFunction = func() time.Time {
		return flushTime
	}

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	packetData, err := testutil.GetNetFlow5Packet()
	require.NoError(t, err, "error getting packet")
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	testutil.ExpectNetflow5Payloads(t, epForwarder)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, time.Second, 2)
	assert.Equal(t, uint64(2), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9(t *testing.T) {
	port := testutil.GetFreePort()
	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	srv := fxutil.Test[Component](t, fx.Options(
		log.MockModule,
		testModule,
		fx.Replace(
			fx.Annotate(epForwarder, fx.As(new(netflowEPForwarder))),
			singleListenerConfig("netflow9", port),
		),
	)).(*Server)

	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

	// Setup NetFlow Server
	srv.FlowAgg.TimeNowFunction = func() time.Time {
		return flushTime
	}

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
	port := testutil.GetFreePort()
	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	srv := fxutil.Test[Component](t, fx.Options(
		log.MockModule,
		testModule,
		fx.Replace(
			fx.Annotate(epForwarder, fx.As(new(netflowEPForwarder))),
			singleListenerConfig("sflow5", port),
		),
	)).(*Server)

	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

	// Setup NetFlow Server
	srv.FlowAgg.TimeNowFunction = func() time.Time {
		return flushTime
	}

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	data, err := testutil.GetSFlow5Packet()
	require.NoError(t, err, "error getting sflow data")

	err = testutil.SendUDPPacket(port, data)
	require.NoError(t, err, "error sending udp packet")

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(srv.FlowAgg, time.Second, 6)
	assert.Equal(t, uint64(7), netflowEvents)
	assert.NoError(t, err)
}
