// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package netflow

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/util/hostname"

	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
)

func TestNewNetflowServer(t *testing.T) {
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

	// Setup NetFlow Server
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(1 * time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	server, err := NewNetflowServer(sender)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	// Send netflowV5Data twice to test aggregator
	// Flows will have 2x bytes/packets after aggregation
	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	now := time.Now()
	mockNetflowPayload := testutil.GenerateNetflow5Packet(now, 6)
	err = testutil.SendUDPPacket(port, testutil.BuildNetFlow5Payload(mockNetflowPayload))
	require.NoError(t, err, "error sending udp packet")

	// Get Event Platform Events
	netflowEvents, err := demux.WaitEventPlatformEvents(epforwarder.EventTypeNetworkDevicesNetFlow, 6, 15*time.Second)
	require.NoError(t, err, "error waiting event platform events")
	assert.Equal(t, 6, len(netflowEvents))

	actualFlow, err := findEventBySourceDest(netflowEvents, "10.0.0.1", "20.0.0.1")
	assert.NoError(t, err)

	assert.Equal(t, "netflow5", actualFlow.FlowType)
	assert.Equal(t, uint64(0), actualFlow.SamplingRate)
	assert.Equal(t, "ingress", actualFlow.Direction)
	assert.Equal(t, uint64(now.Unix()), actualFlow.Start)
	assert.Equal(t, uint64(now.Unix()), actualFlow.End)
	assert.Equal(t, uint64(194), actualFlow.Bytes)
	assert.Equal(t, uint64(10), actualFlow.Packets)
	assert.Equal(t, "IPv4", actualFlow.EtherType)
	assert.Equal(t, "TCP", actualFlow.IPProtocol)
	assert.Equal(t, "127.0.0.1", actualFlow.Device.IP)
	assert.Equal(t, "10.0.0.1", actualFlow.Source.IP)
	assert.Equal(t, "50000", actualFlow.Source.Port)
	assert.Equal(t, "00:00:00:00:00:00", actualFlow.Source.Mac)
	assert.Equal(t, "0.0.0.0/0", actualFlow.Source.Mask)
	assert.Equal(t, "20.0.0.1", actualFlow.Destination.IP)
	assert.Equal(t, "8080", actualFlow.Destination.Port)
	assert.Equal(t, "00:00:00:00:00:00", actualFlow.Destination.Mac)
	assert.Equal(t, "0.0.0.0/0", actualFlow.Destination.Mask)
	assert.Equal(t, uint32(1), actualFlow.Ingress.Interface.Index)
	assert.Equal(t, uint32(7), actualFlow.Egress.Interface.Index)
	assert.Equal(t, "default", actualFlow.Device.Namespace)
	hostnameDetected, _ := hostname.Get(context.TODO())
	assert.Equal(t, hostnameDetected, actualFlow.Host)
	assert.ElementsMatch(t, []string{"SYN", "RST", "ACK"}, actualFlow.TCPFlags)
	assert.Equal(t, "0.0.0.0", actualFlow.NextHop.IP)
}

func TestStartServerAndStopServer(t *testing.T) {
	demux := aggregator.InitTestAgentDemultiplexerWithFlushInterval(10 * time.Millisecond)
	defer demux.Stop(false)

	sender, err := demux.GetDefaultSender()
	require.NoError(t, err, "cannot get default sender")

	err = StartServer(sender)
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

	server, err := NewNetflowServer(sender)
	require.NoError(t, err, "cannot start Netflow Server")
	assert.NotNil(t, server)

	flowProcessor := replaceWithDummyFlowProcessor(server, port)

	// Stops server
	server.stop()

	// Assert logs present
	assert.Equal(t, flowProcessor.stopped, true)
}
