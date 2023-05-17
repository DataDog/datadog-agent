// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus"
	promClient "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/epforwarder"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"

	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/pkg/netflow/testutil"
)

func TestAggregator(t *testing.T) {
	stoppedMu := sync.RWMutex{} // Mutex needed to avoid race condition in test

	sender := mocksender.NewMockSender("")
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}
	flow := &common.Flow{
		Namespace:      "my-ns",
		FlowType:       common.TypeNetFlow9,
		ExporterAddr:   []byte{127, 0, 0, 1},
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        []byte{10, 10, 10, 10},
		DstAddr:        []byte{10, 10, 10, 20},
		IPProtocol:     uint32(6),
		SrcPort:        2000,
		DstPort:        80,
		TCPFlags:       19,
		EtherType:      uint32(0x0800),
	}
	epForwarder := epforwarder.NewMockEventPlatformForwarder(gomock.NewController(t))

	// language=json
	event := []byte(`
{
  "type": "netflow9",
  "sampling_rate": 0,
  "direction": "ingress",
  "start": 1234568,
  "end": 1234569,
  "bytes": 20,
  "packets": 4,
  "ether_type": "IPv4",
  "ip_protocol": "TCP",
  "device": {
    "namespace": "my-ns"
  },
  "exporter": {
    "ip": "127.0.0.1"
  },
  "source": {
    "ip": "10.10.10.10",
    "port": "2000",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0"
  },
  "destination": {
    "ip": "10.10.10.20",
    "port": "80",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0"
  },
  "ingress": {
    "interface": {
      "index": 0
    }
  },
  "egress": {
    "interface": {
      "index": 0
    }
  },
  "host": "my-hostname",
  "tcp_flags": [
    "FIN",
    "SYN",
    "ACK"
  ],
  "next_hop": {
    "ip": ""
  }
}
`)
	compactEvent := new(bytes.Buffer)
	err := json.Compact(compactEvent, event)
	assert.NoError(t, err)

	// language=json
	metadataEvent := []byte(fmt.Sprintf(`
{
  "namespace":"my-ns",
  "netflow_exporters":[
    {
      "id": "my-ns:127.0.0.1:netflow9",
      "ip_address":"127.0.0.1",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1550505606
}
`))
	compactMetadataEvent := new(bytes.Buffer)
	err = json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactEvent.Bytes()}, "network-devices-netflow").Return(nil).Times(1)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")
	aggregator.flushFlowsToSendInterval = 1 * time.Second
	aggregator.timeNowFunction = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")
		return t
	}
	inChan := aggregator.GetFlowInChan()

	expectStartExisted := false
	go func() {
		aggregator.Start()
		stoppedMu.Lock()
		expectStartExisted = true
		stoppedMu.Unlock()
	}()
	inChan <- flow

	netflowEvents, err := WaitForFlowsToBeFlushed(aggregator, 10*time.Second, 1)
	assert.NoError(t, err)
	assert.Equal(t, uint64(1), netflowEvents)

	sender.AssertMetric(t, "Count", "datadog.netflow.aggregator.flows_flushed", 1, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.aggregator.flows_received", 1, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.flows_contexts", 1, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.current_store_size", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.new_store_size", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.capacity", 20, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.length", 0, "", nil)

	// Test aggregator Stop
	assert.False(t, expectStartExisted)
	aggregator.Stop()

	waitStopTimeout := time.After(2 * time.Second)
	waitStopTick := time.Tick(100 * time.Millisecond)
stopLoop:
	for {
		select {
		case <-waitStopTimeout:
			assert.Fail(t, "timeout waiting for aggregator to be stopped")
		case <-waitStopTick:
			stoppedMu.Lock()
			startExited := expectStartExisted
			stoppedMu.Unlock()
			if startExited {
				break stopLoop
			}
		}
	}
}

func TestAggregator_withMockPayload(t *testing.T) {
	port := uint16(52056)

	sender := mocksender.NewMockSender("")
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(port),
				Workers:  10,
			},
		},
	}
	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	testutil.ExpectNetflow5Payloads(t, epForwarder)

	// language=json
	metadataEvent := []byte(fmt.Sprintf(`
{
  "namespace":"default",
  "netflow_exporters":[
    {
      "id": "default:127.0.0.1:netflow5",
      "ip_address":"127.0.0.1",
      "flow_type":"netflow5"
    }
  ],
  "collect_timestamp": 1550505606
}
`))
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	require.NoError(t, err)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")
	aggregator.flushFlowsToSendInterval = 1 * time.Second
	aggregator.timeNowFunction = func() time.Time {
		t, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")
		return t
	}

	stoppedFlushLoop := make(chan struct{})
	stoppedRun := make(chan struct{})
	go func() {
		aggregator.run()
		stoppedRun <- struct{}{}
	}()
	go func() {
		aggregator.flushLoop()
		stoppedFlushLoop <- struct{}{}
	}()

	flowState, err := goflowlib.StartFlowRoutine(common.TypeNetFlow5, "127.0.0.1", port, 1, "default", aggregator.GetFlowInChan())
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	packetData, err := testutil.GetNetFlow5Packet()
	require.NoError(t, err, "error getting packet")
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	netflowEvents, err := WaitForFlowsToBeFlushed(aggregator, 3*time.Second, 2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), netflowEvents)

	sender.AssertMetric(t, "Count", "datadog.netflow.aggregator.flows_flushed", 2, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.aggregator.flows_received", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.flows_contexts", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.current_store_size", 4, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.new_store_size", 4, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.capacity", 20, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.length", 0, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.decoder.messages", 1, "", []string{"collector_type:netflow5", "worker:0"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.flows", 1, "", []string{"exporter_ip:127.0.0.1", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.flowsets", 2, "", []string{"exporter_ip:127.0.0.1", "type:data_flow_set", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.bytes", 120, "", []string{"listener_port:52056", "exporter_ip:127.0.0.1", "collector_type:netflow5"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.packets", 1, "", []string{"listener_port:52056", "exporter_ip:127.0.0.1", "collector_type:netflow5"})

	flowState.Shutdown()
	aggregator.Stop()

	<-stoppedFlushLoop
	<-stoppedRun
}

func TestFlowAggregator_flush_submitCollectorMetrics_error(t *testing.T) {
	// 1/ Arrange
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := seelog.LoggerFromWriterWithMinLevelAndFormat(w, seelog.DebugLvl, "[%LEVEL] %FuncShort: %Msg")
	assert.Nil(t, err)
	log.SetupLogger(l, "debug")

	sender := mocksender.NewMockSender("")
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return nil, fmt.Errorf("some prometheus gatherer error")
	})

	// 2/ Act
	aggregator.flush()

	// 3/ Assert
	w.Flush()
	logs := b.String()
	assert.Equal(t, strings.Count(logs, "[WARN] flush: error submitting collector metrics: some prometheus gatherer error"), 1, logs)
}

func TestFlowAggregator_submitCollectorMetrics(t *testing.T) {
	sender := mocksender.NewMockSender("")
	sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return []*promClient.MetricFamily{
			{
				Name: proto.String("flow_decoder_count"),
				Type: promClient.MetricType_COUNTER.Enum(),
				Metric: []*promClient.Metric{
					{
						Counter: &promClient.Counter{Value: proto.Float64(10)},
						Label: []*promClient.LabelPair{
							{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
							{Name: proto.String("worker"), Value: proto.String("1")},
						},
					},
				},
			},
			{
				Name: proto.String("flow_decoder_error_count"),
				Type: promClient.MetricType_GAUGE.Enum(),
				Metric: []*promClient.Metric{
					{
						Gauge: &promClient.Gauge{Value: proto.Float64(20)},
						Label: []*promClient.LabelPair{
							{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
							{Name: proto.String("worker"), Value: proto.String("2")},
						},
					},
				},
			},
			{
				Name: proto.String("flow_decoder_error_count"),
				Type: promClient.MetricType_UNTYPED.Enum(), // unsupported type is ignored
				Metric: []*promClient.Metric{
					{
						Gauge: &promClient.Gauge{Value: proto.Float64(20)},
						Label: []*promClient.LabelPair{
							{Name: proto.String("name"), Value: proto.String("NetFlowV5")},
							{Name: proto.String("worker"), Value: proto.String("2")},
						},
					},
				},
			},
		}, nil
	})

	// 2/ Act
	err := aggregator.submitCollectorMetrics()
	assert.NoError(t, err)

	// 3/ Assert
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.decoder.messages", 10, "", []string{"collector_type:netflow5", "worker:1"})
	sender.AssertMetric(t, "Gauge", "datadog.netflow.decoder.errors", 20.0, "", []string{"collector_type:netflow5", "worker:2"})
}

func TestFlowAggregator_submitCollectorMetrics_error(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return nil, fmt.Errorf("some prometheus gatherer error")
	})

	// 2/ Act
	err := aggregator.submitCollectorMetrics()

	// 3/ Assert
	assert.EqualError(t, err, "some prometheus gatherer error")
}

func TestFlowAggregator_sendExporterMetadata_multiplePayloads(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")

	var flows []*common.Flow
	for i := 1; i <= 250; i++ {
		flows = append(flows, &common.Flow{
			Namespace:      "my-ns",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, byte(i)},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		})
	}
	now := time.Unix(1681295467, 0)
	var payload1NetflowExporters []metadata.NetflowExporter
	for i := 1; i <= 100; i++ {
		payload1NetflowExporters = append(payload1NetflowExporters, metadata.NetflowExporter{
			ID:        fmt.Sprintf("my-ns:127.0.0.%d:netflow9", i),
			IPAddress: "127.0.0." + strconv.Itoa(i),
			FlowType:  "netflow9",
		})
	}
	var payload2NetflowExporters []metadata.NetflowExporter
	for i := 101; i <= 200; i++ {
		payload2NetflowExporters = append(payload2NetflowExporters, metadata.NetflowExporter{
			ID:        fmt.Sprintf("my-ns:127.0.0.%d:netflow9", i),
			IPAddress: "127.0.0." + strconv.Itoa(i),
			FlowType:  "netflow9",
		})
	}
	var payload3NetflowExporters []metadata.NetflowExporter
	for i := 201; i <= 250; i++ {
		payload3NetflowExporters = append(payload3NetflowExporters, metadata.NetflowExporter{
			ID:        fmt.Sprintf("my-ns:127.0.0.%d:netflow9", i),
			IPAddress: "127.0.0." + strconv.Itoa(i),
			FlowType:  "netflow9",
		})
	}
	for _, exporters := range [][]metadata.NetflowExporter{payload1NetflowExporters, payload2NetflowExporters, payload3NetflowExporters} {
		payload := metadata.NetworkDevicesMetadata{
			Subnet:           "",
			Namespace:        "my-ns",
			CollectTimestamp: now.Unix(),
			NetflowExporters: exporters,
		}
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)

		m := &message.Message{Content: payloadBytes}
		epForwarder.EXPECT().SendEventPlatformEventBlocking(m, "network-devices-metadata").Return(nil).Times(1)
	}
	aggregator.sendExporterMetadata(flows, now)
}

func TestFlowAggregator_sendExporterMetadata_noPayloads(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType: common.TypeNetFlow9,
				BindHost: "127.0.0.1",
				Port:     uint16(1234),
				Workers:  10,
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")

	var flows []*common.Flow
	now := time.Unix(1681295467, 0)

	// call sendExporterMetadata does not trigger any call to epForwarder.SendEventPlatformEventBlocking(...)
	aggregator.sendExporterMetadata(flows, now)
}

func TestFlowAggregator_sendExporterMetadata_invalidIPIgnored(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType:  common.TypeNetFlow9,
				BindHost:  "127.0.0.1",
				Port:      uint16(1234),
				Workers:   10,
				Namespace: "my-ns",
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")

	now := time.Unix(1681295467, 0)
	flows := []*common.Flow{
		{
			Namespace:      "my-ns",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{99}, // INVALID ADDR
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
		{
			Namespace:      "my-ns",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 10},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
	}

	// language=json
	metadataEvent := []byte(fmt.Sprintf(`
{
  "namespace":"my-ns",
  "netflow_exporters":[
    {
      "id": "my-ns:127.0.0.10:netflow9",
      "ip_address":"127.0.0.10",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`))
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	// call sendExporterMetadata does not trigger any call to epForwarder.SendEventPlatformEventBlocking(...)
	aggregator.sendExporterMetadata(flows, now)
}

func TestFlowAggregator_sendExporterMetadata_multipleNamespaces(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType:  common.TypeNetFlow9,
				BindHost:  "127.0.0.1",
				Port:      uint16(1234),
				Workers:   10,
				Namespace: "my-ns",
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")

	now := time.Unix(1681295467, 0)
	flows := []*common.Flow{
		{
			Namespace:      "my-ns1",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 11},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
		{
			Namespace:      "my-ns2",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 12},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
	}

	// language=json
	metadataEvent := []byte(fmt.Sprintf(`
{
  "namespace":"my-ns1",
  "netflow_exporters":[
    {
      "id": "my-ns1:127.0.0.11:netflow9",
      "ip_address":"127.0.0.11",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`))
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	// language=json
	metadataEvent2 := []byte(fmt.Sprintf(`
{
  "namespace":"my-ns2",
  "netflow_exporters":[
    {
      "id": "my-ns2:127.0.0.12:netflow9",
      "ip_address":"127.0.0.12",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`))
	compactMetadataEvent2 := new(bytes.Buffer)
	err = json.Compact(compactMetadataEvent2, metadataEvent2)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent2.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	// call sendExporterMetadata does not trigger any call to epForwarder.SendEventPlatformEventBlocking(...)
	aggregator.sendExporterMetadata(flows, now)
}

func TestFlowAggregator_sendExporterMetadata_singleExporterIpWithMultipleFlowTypes(t *testing.T) {
	sender := mocksender.NewMockSender("")
	conf := config.NetflowConfig{
		StopTimeout:                            10,
		AggregatorBufferSize:                   20,
		AggregatorFlushInterval:                1,
		AggregatorPortRollupThreshold:          10,
		AggregatorRollupTrackerRefreshInterval: 3600,
		Listeners: []config.ListenerConfig{
			{
				FlowType:  common.TypeNetFlow9,
				BindHost:  "127.0.0.1",
				Port:      uint16(1234),
				Workers:   10,
				Namespace: "my-ns",
			},
		},
	}

	ctrl := gomock.NewController(t)
	epForwarder := epforwarder.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname")

	now := time.Unix(1681295467, 0)
	flows := []*common.Flow{
		{
			Namespace:      "my-ns1",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 11},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
		{
			Namespace:      "my-ns1",
			FlowType:       common.TypeNetFlow5,
			ExporterAddr:   []byte{127, 0, 0, 11},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          20,
			Packets:        4,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			TCPFlags:       19,
			EtherType:      uint32(0x0800),
		},
	}

	// language=json
	metadataEvent := []byte(fmt.Sprintf(`
{
  "namespace":"my-ns1",
  "netflow_exporters":[
    {
      "id": "my-ns1:127.0.0.11:netflow9",
      "ip_address":"127.0.0.11",
      "flow_type":"netflow9"
    },
    {
      "id": "my-ns1:127.0.0.11:netflow5",
      "ip_address":"127.0.0.11",
      "flow_type":"netflow5"
    }
  ],
  "collect_timestamp": 1681295467
}
`))
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(&message.Message{Content: compactMetadataEvent.Bytes()}, "network-devices-metadata").Return(nil).Times(1)

	// call sendExporterMetadata does not trigger any call to epForwarder.SendEventPlatformEventBlocking(...)
	aggregator.sendExporterMetadata(flows, now)
}
