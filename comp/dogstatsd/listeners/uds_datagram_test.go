// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build !windows

// UDS won't work in windows

package listeners

import (
	"net"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/comp/core/config"
	"github.com/DataDog/datadog-agent/comp/core/telemetry"
	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/packets"
	"github.com/DataDog/datadog-agent/comp/dogstatsd/pidmap"
	"github.com/DataDog/datadog-agent/pkg/util/optional"
)

func udsDatagramListenerFactory(packetOut chan packets.Packets, manager *packets.PoolManager[packets.Packet], cfg config.Component, pidMap pidmap.Component, telemetryStore *TelemetryStore, packetsTelemetryStore *packets.TelemetryStore, telemetry telemetry.Component) (StatsdListener, error) {
	return NewUDSDatagramListener(packetOut, manager, nil, cfg, nil, optional.NewNoneOption[workloadmeta.Component](), pidMap, telemetryStore, packetsTelemetryStore, telemetry)
}

func TestNewUDSDatagramListener(t *testing.T) {
	testNewUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestStartStopUDSDatagramListener(t *testing.T) {
	testStartStopUDSListener(t, udsDatagramListenerFactory, "unixgram")
}

func TestUDSDatagramReceive(t *testing.T) {
	socketPath := testSocketPath(t)

	mockConfig := map[string]interface{}{}
	mockConfig[socketPathConfKey("unixgram")] = socketPath
	mockConfig["dogstatsd_origin_detection"] = false

	var contents0 = []byte("daemon:666|g|#sometag1:somevalue1,sometag2:somevalue2")
	var contents1 = []byte("daemon:999|g|#sometag1:somevalue1")

	packetsChannel := make(chan packets.Packets)

	deps := fulfillDepsWithConfig(t, mockConfig)
	telemetryStore := NewTelemetryStore(nil, deps.Telemetry)
	packetsTelemetryStore := packets.NewTelemetryStore(nil, deps.Telemetry)
	s, err := udsDatagramListenerFactory(packetsChannel, newPacketPoolManagerUDS(deps.Config, packetsTelemetryStore), deps.Config, deps.PidMap, telemetryStore, packetsTelemetryStore, deps.Telemetry)
	assert.Nil(t, err)
	assert.NotNil(t, s)

	s.Listen()
	defer s.Stop()
	conn, err := net.Dial("unixgram", socketPath)
	assert.Nil(t, err)
	defer conn.Close()

	conn.Write([]byte{})
	conn.Write(contents0)
	conn.Write(contents1)

	select {
	case pkts := <-packetsChannel:
		assert.Equal(t, 3, len(pkts))

		packet := pkts[0]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, []byte{})
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[1]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents0)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		packet = pkts[2]
		assert.NotNil(t, packet)
		assert.Equal(t, packet.Contents, contents1)
		assert.Equal(t, packet.Origin, "")
		assert.Equal(t, packet.Source, packets.UDS)

		telemetryMock, ok := deps.Telemetry.(telemetry.Mock)
		assert.True(t, ok)

		udsConnectionsMetrics, err := telemetryMock.GetGaugeMetric("dogstatsd", "uds_connections")
		require.NoError(t, err)
		require.Len(t, udsConnectionsMetrics, 1)

		udsPacketsMetrics, err := telemetryMock.GetCountMetric("dogstatsd", "uds_packets")
		require.NoError(t, err)
		require.Len(t, udsPacketsMetrics, 1)

		udsPacketsBytesMetrics, err := telemetryMock.GetCountMetric("dogstatsd", "uds_packets_bytes")
		require.NoError(t, err)
		require.Len(t, udsPacketsBytesMetrics, 1)

		readLatencyMetrics, err := telemetryMock.GetHistogramMetric("dogstatsd", "listener_read_latency")
		require.NoError(t, err)
		require.Len(t, readLatencyMetrics, 1)

		udsConnectionsMetricLabel := udsConnectionsMetrics[0].Tags()
		assert.Equal(t, udsConnectionsMetricLabel["listener_id"], "uds-unixgram")
		assert.Equal(t, udsConnectionsMetricLabel["transport"], "unixgram")

		assert.Equal(t, float64(1), udsConnectionsMetrics[0].Value())

		readLatencyMetricLabel := readLatencyMetrics[0].Tags()
		assert.Equal(t, readLatencyMetricLabel["listener_id"], "uds-unixgram")
		assert.Equal(t, readLatencyMetricLabel["listener_type"], "uds")
		assert.Equal(t, readLatencyMetricLabel["transport"], "unixgram")
		assert.NotEqual(t, float64(0), readLatencyMetrics[0].Value())

		udsPacketsMetricLabel := udsPacketsMetrics[0].Tags()
		assert.Equal(t, udsPacketsMetricLabel["listener_id"], "uds-unixgram")
		assert.Equal(t, udsPacketsMetricLabel["state"], "ok")
		assert.Equal(t, udsPacketsMetricLabel["transport"], "unixgram")
		assert.Equal(t, float64(3), udsPacketsMetrics[0].Value())

		udsPacketsBytesMetricLabel := udsPacketsBytesMetrics[0].Tags()
		assert.Equal(t, udsPacketsBytesMetricLabel["listener_id"], "uds-unixgram")
		assert.Equal(t, udsPacketsBytesMetricLabel["transport"], "unixgram")
		assert.Equal(t, float64(86), udsPacketsBytesMetrics[0].Value())
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
