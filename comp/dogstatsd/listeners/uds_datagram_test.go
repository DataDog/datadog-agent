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

	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"

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

		registry := telemetryMock.GetRegistry()
		var udsConnectionsMetric []*dto.Metric
		var udsPacketsMetric []*dto.Metric
		var udsPacketsBytesMetric []*dto.Metric
		var readLatencyMetric []*dto.Metric

		metricsFamily, err := registry.Gather()
		assert.Nil(t, err)

		for _, metric := range metricsFamily {
			if metric.GetName() == "dogstatsd__listener_read_latency" {
				readLatencyMetric = metric.GetMetric()
			}
			if metric.GetName() == "dogstatsd__uds_connections" {
				udsConnectionsMetric = metric.GetMetric()
			}
			if metric.GetName() == "dogstatsd__uds_packets" {
				udsPacketsMetric = metric.GetMetric()
			}
			if metric.GetName() == "dogstatsd__uds_packets_bytes" {
				udsPacketsBytesMetric = metric.GetMetric()
			}
		}

		assert.NotNil(t, readLatencyMetric)
		assert.NotNil(t, udsConnectionsMetric)
		assert.NotNil(t, udsPacketsMetric)
		assert.NotNil(t, udsPacketsBytesMetric)

		udsConnectionsMetricLabel := udsConnectionsMetric[0].GetLabel()
		assert.Equal(t, "listener_id", udsConnectionsMetricLabel[0].GetName())
		assert.Equal(t, "uds-unixgram", udsConnectionsMetricLabel[0].GetValue())
		assert.Equal(t, "transport", udsConnectionsMetricLabel[1].GetName())
		assert.Equal(t, "unixgram", udsConnectionsMetricLabel[1].GetValue())
		// For each packet we increment and decrement the counter, so the counter being at zero means we received all the packets
		assert.Equal(t, float64(0), udsConnectionsMetric[0].GetCounter().GetValue())

		readLatencyMetricLabel := readLatencyMetric[0].GetLabel()
		assert.Equal(t, "listener_id", readLatencyMetricLabel[0].GetName())
		assert.Equal(t, "uds-unixgram", readLatencyMetricLabel[0].GetValue())
		assert.Equal(t, "listener_type", readLatencyMetricLabel[1].GetName())
		assert.Equal(t, "uds", readLatencyMetricLabel[1].GetValue())
		assert.Equal(t, "transport", readLatencyMetricLabel[2].GetName())
		assert.Equal(t, "unixgram", readLatencyMetricLabel[2].GetValue())
		assert.NotEqual(t, float64(0), readLatencyMetric[0].GetHistogram().GetSampleSum())

		udsPacketsMetricLabel := udsPacketsMetric[0].GetLabel()
		assert.Equal(t, "listener_id", udsPacketsMetricLabel[0].GetName())
		assert.Equal(t, "uds-unixgram", udsPacketsMetricLabel[0].GetValue())
		assert.Equal(t, "state", udsPacketsMetricLabel[1].GetName())
		assert.Equal(t, "ok", udsPacketsMetricLabel[1].GetValue())
		assert.Equal(t, "transport", udsPacketsMetricLabel[2].GetName())
		assert.Equal(t, "unixgram", udsPacketsMetricLabel[2].GetValue())
		assert.Equal(t, float64(3), udsPacketsMetric[0].GetCounter().GetValue())

		udsPacketsBytesMetricLabel := udsPacketsBytesMetric[0].GetLabel()
		assert.Equal(t, "listener_id", udsPacketsBytesMetricLabel[0].GetName())
		assert.Equal(t, "uds-unixgram", udsPacketsBytesMetricLabel[0].GetValue())
		assert.Equal(t, "transport", udsPacketsBytesMetricLabel[1].GetName())
		assert.Equal(t, "unixgram", udsPacketsBytesMetricLabel[1].GetValue())
		assert.Equal(t, float64(86), udsPacketsBytesMetric[0].GetCounter().GetValue())
	case <-time.After(2 * time.Second):
		assert.FailNow(t, "Timeout on receive channel")
	}

}
