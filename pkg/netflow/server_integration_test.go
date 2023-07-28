// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package netflow

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/log"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
)

func TestNetFlow_IntegrationTest_NetFlow5(t *testing.T) {
	// Setup NetFlow feature config
	port := testutil.GetFreePort()
	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: netflow5 # netflow, sflow, ipfix
        bind_host: 127.0.0.1
        port: %d # default 2055 for netflow
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	// Setup NetFlow Server
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 1*time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	server.flowAgg.TimeNowFunction = func() time.Time {
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

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 2)
	assert.Equal(t, uint64(2), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9(t *testing.T) {
	// Setup NetFlow feature config
	port := testutil.GetFreePort()
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: netflow9 # netflow, sflow, ipfix
        bind_host: 127.0.0.1
        port: %d # default 2055 for netflow
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	// Setup NetFlow Server
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 1*time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	packetData, err := testutil.GetNetFlow9Packet()
	require.NoError(t, err, "error getting packet")
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_NetFlow9_investigation(t *testing.T) {
	// Setup NetFlow feature config
	port := testutil.GetFreePort()
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: netflow9 # netflow, sflow, ipfix
        bind_host: 127.0.0.1
        port: %d # default 2055 for netflow
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	// Setup NetFlow Server
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 1*time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	for _, packetIndex := range []int{4, 5} {
		packetData, err := testutil.GetNetFlow9InvestigationPacket(packetIndex)
		require.NoError(t, err, "error getting packet")
		err = testutil.SendUDPPacket(port, packetData)
		require.NoError(t, err, "error sending udp packet")
	}

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNetFlow_IntegrationTest_SFlow5(t *testing.T) {
	// Setup NetFlow feature config
	port := testutil.GetFreePort()
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: sflow5 # netflow, sflow, ipfix
        bind_host: 127.0.0.1
        port: %d
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	// Setup NetFlow Server
	log := fxutil.Test[log.Component](t, log.MockModule)
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(log, 1*time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	data, err := testutil.GetSFlow5Packet()
	require.NoError(t, err, "error getting sflow data")

	err = testutil.SendUDPPacket(port, data)
	require.NoError(t, err, "error sending udp packet")

	// Test later content of payloads if needed for more precise test.
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(7), netflowEvents)
	assert.NoError(t, err)
}
