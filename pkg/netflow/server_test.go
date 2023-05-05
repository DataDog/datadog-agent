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

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/netflow/flowaggregator"
	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
)

func TestNewNetflowServer_NetFlow5(t *testing.T) {
	// Setup NetFlow feature config
	port := uint16(52055)
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
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(1 * time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	now := time.Unix(1494505756, 0)
	mockNetflowPayload := testutil.GenerateNetflow5Packet(now, 6)
	err = testutil.SendUDPPacket(port, testutil.BuildNetFlow5Payload(mockNetflowPayload))
	require.NoError(t, err, "error sending udp packet")

	testutil.ExpectNetflow5Payloads(t, epForwarder, now, "my-hostname", 6)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(6), netflowEvents)
	assert.NoError(t, err)
}

func TestNewNetflowServer_NetFlow9(t *testing.T) {
	// Setup NetFlow feature config
	port := uint16(52056)
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
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(1 * time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	err = testutil.SendUDPPacket(port, testutil.Netflow9PayloadWithTemplateAndData)
	require.NoError(t, err, "error sending udp packet")

	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(29)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(29), netflowEvents)
	assert.NoError(t, err)
}

func TestNewNetflowServer_SFlow5(t *testing.T) {
	// Setup NetFlow feature config
	port := uint16(52057)
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
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(1 * time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)
	server, err := NewNetflowServer(sender, epForwarder)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	err = testutil.SendUDPPacket(port, testutil.SFlow5Payload)
	require.NoError(t, err, "error sending udp packet")

	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), epforwarder.EventTypeNetworkDevicesNetFlow).Return(nil).Times(7)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), "network-devices-metadata").Return(nil).Times(1)

	netflowEvents, err := flowaggregator.WaitForFlowsToBeFlushed(server.flowAgg, 15*time.Second, 6)
	assert.Equal(t, uint64(7), netflowEvents)
	assert.NoError(t, err)
}

func TestStartServerAndStopServer(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)

	port := uint16(52056)
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    listeners:
      - flow_type: netflow5
        bind_host: 127.0.0.1
        port: %d
`, port)))
	require.NoError(t, err)
	config.Datadog.Set("hostname", "my-hostname")

	err = StartServer(demux)
	require.NoError(t, err)
	require.NotNil(t, serverInstance)

	replaceWithDummyFlowProcessor(serverInstance, 123)

	StopServer()
	require.Nil(t, serverInstance)
}

func TestIsEnabled(t *testing.T) {
	saved := config.Datadog.Get("network_devices.netflow.enabled")
	defer config.Datadog.Set("network_devices.netflow.enabled", saved)

	config.Datadog.Set("network_devices.netflow.enabled", true)
	assert.Equal(t, true, IsEnabled())

	config.Datadog.Set("network_devices.netflow.enabled", false)
	assert.Equal(t, false, IsEnabled())
}

func TestServer_Stop(t *testing.T) {
	// Setup NetFlow config
	port := uint16(12056)
	config.Datadog.SetConfigType("yaml")
	err := config.Datadog.MergeConfigOverride(strings.NewReader(fmt.Sprintf(`
network_devices:
  netflow:
    enabled: true
    aggregator_flush_interval: 1
    listeners:
      - flow_type: netflow5 # netflow, sflow, ipfix
        bind_host: 0.0.0.0
        port: %d # default 2055 for netflow
`, port)))
	require.NoError(t, err)

	// Setup Netflow Server
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)
	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	server, err := NewNetflowServer(sender, nil)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	flowProcessor := replaceWithDummyFlowProcessor(server, port)

	// Stops server
	server.stop()

	// Assert logs present
	assert.Equal(t, flowProcessor.stopped, true)
}
