// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

//go:build test

package flowaggregator

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gogo/protobuf/proto"
	"github.com/golang/mock/gomock"
	"github.com/prometheus/client_golang/prometheus"
	promClient "github.com/prometheus/client_model/go"
	"go.uber.org/atomic"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/comp/forwarder/eventplatform/eventplatformimpl"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/logs/message"
	"github.com/DataDog/datadog-agent/pkg/util/fxutil"
	ddlog "github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/networkdevice/metadata"
	ndmtestutils "github.com/DataDog/datadog-agent/pkg/networkdevice/testutils"

	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/DataDog/datadog-agent/comp/netflow/config"
	"github.com/DataDog/datadog-agent/comp/netflow/goflowlib"
	"github.com/DataDog/datadog-agent/comp/netflow/testutil"
	rdnsquerier "github.com/DataDog/datadog-agent/comp/rdnsquerier/def"
	rdnsquerierfxmock "github.com/DataDog/datadog-agent/comp/rdnsquerier/fx-mock"
)

func TestAggregator(t *testing.T) {
	stoppedMu := sync.RWMutex{} // Mutex needed to avoid race condition in test
	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")
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
		AggregatorMaxFlowsPerPeriod:            0,
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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(gomock.NewController(t))

	// language=json
	event := []byte(`
{
  "bytes": 20,
  "destination": {
    "ip": "10.10.10.20",
    "port": "80",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0",
    "reverse_dns_hostname": "hostname-10.10.10.20"
  },
  "device": {
    "namespace": "my-ns"
  },
  "direction": "ingress",
  "egress": {
    "interface": {
      "index": 0
    }
  },
  "end": 1234569,
  "ether_type": "IPv4",
  "exporter": {
    "ip": "127.0.0.1"
  },
  "flush_timestamp": 1550505606000,
  "host": "my-hostname",
  "ingress": {
    "interface": {
      "index": 0
    }
  },
  "ip_protocol": "TCP",
  "next_hop": {
    "ip": ""
  },
  "packets": 4,
  "sampling_rate": 0,
  "source": {
    "ip": "10.10.10.10",
    "port": "2000",
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/0",
    "reverse_dns_hostname": "hostname-10.10.10.10"
  },
  "start": 1234568,
  "tcp_flags": [
    "FIN",
    "SYN",
    "ACK"
  ],
  "type": "netflow9"
}
`)
	compactEvent := new(bytes.Buffer)
	err := json.Compact(compactEvent, event)
	assert.NoError(t, err)

	// language=json
	metadataEvent := []byte(`
{
  "namespace":"my-ns",
  "integration": "netflow",
  "netflow_exporters":[
    {
      "id": "my-ns:127.0.0.1:netflow9",
      "ip_address":"127.0.0.1",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1550505606
}
`)
	compactMetadataEvent := new(bytes.Buffer)
	err = json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactEvent.Bytes(), nil, "", 0), "network-devices-netflow").Return(nil).Times(1)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)
	aggregator.FlushConfig.FlushTickFrequency = 1 * time.Second
	aggregator.TimeNowFunction = func() time.Time {
		return flushTime
	}
	// get hooks into the tickers so we can manually trigger flushes
	flushChannel, _ := SetAggregatorTicker(aggregator)
	// set the timestamp that will be associated with incoming flows
	setMockTimeNow(flushTime.Add(-1 * time.Second))

	inChan := aggregator.GetFlowInChan()

	expectStartExisted := false
	go func() {
		aggregator.Start()
		stoppedMu.Lock()
		expectStartExisted = true
		stoppedMu.Unlock()
	}()
	inChan <- flow

	// wait for flows to be processed by the channel
	err = WaitForFlowsToAccumulate(aggregator, 5*time.Second, 1)
	require.NoError(t, err, "we need the flow to be accumulated")

	// trigger a flush by publishing a timestamp to the channel
	flushChannel <- flushTime

	// wait for the flush to complete and assert
	netflowEvents, err := WaitForFlowsToBeFlushed(aggregator, 5*time.Second, 1)
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
	port, err := ndmtestutils.GetFreePort()
	require.NoError(t, err)
	flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:06Z")

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

	testutil.ExpectNetflow5Payloads(t, epForwarder)

	// language=json
	metadataEvent := []byte(`
{
  "namespace":"default",
  "integration": "netflow",
  "netflow_exporters":[
    {
      "id": "default:127.0.0.1:netflow5",
      "ip_address":"127.0.0.1",
      "flow_type":"netflow5"
    }
  ],
  "collect_timestamp": 1550505606
}
`)
	compactMetadataEvent := new(bytes.Buffer)
	err = json.Compact(compactMetadataEvent, metadataEvent)
	require.NoError(t, err)

	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)

	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)
	aggregator.FlushConfig.FlushTickFrequency = 1 * time.Second
	aggregator.TimeNowFunction = func() time.Time {
		return flushTime
	}
	flushChannel, _ := SetAggregatorTicker(aggregator)
	setMockTimeNow(flushTime)

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

	// Create an error channel to pass to StartFlowRoutine
	listenerErr := atomic.NewString("")
	listenerFlowCount := atomic.NewInt64(0)

	flowState, err := goflowlib.StartFlowRoutine(common.TypeNetFlow5, "127.0.0.1", port, 1, "default", nil, aggregator.GetFlowInChan(), logger, listenerErr, listenerFlowCount)
	assert.NoError(t, err)

	time.Sleep(100 * time.Millisecond) // wait to make sure goflow listener is started before sending

	packetData, err := testutil.GetNetFlow5Packet()
	require.NoError(t, err, "error getting packet")
	err = testutil.SendUDPPacket(port, packetData)
	require.NoError(t, err, "error sending udp packet")

	err = WaitForFlowsToAccumulate(aggregator, 1500*time.Millisecond, 2)
	require.NoError(t, err, "flows must be accumulated before flushing")

	flushChannel <- flushTime

	netflowEvents, err := WaitForFlowsToBeFlushed(aggregator, 1500*time.Millisecond, 2)
	assert.NoError(t, err)
	assert.Equal(t, uint64(2), netflowEvents)

	sender.AssertMetric(t, "Count", "datadog.netflow.aggregator.flows_flushed", 2, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.aggregator.flows_received", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.flows_contexts", 2, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.current_store_size", 4, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.new_store_size", 4, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.capacity", 20, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.length", 0, "", nil)
	sender.AssertMetric(t, "Count", "datadog.netflow.aggregator.sequence.delta", 0, "", []string{"exporter_ip:127.0.0.1", "device_namespace:default", "flow_type:netflow5"})
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.sequence.last", 94, "", []string{"exporter_ip:127.0.0.1", "device_namespace:default", "flow_type:netflow5"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.decoder.messages", 1, "", []string{"collector_type:netflow5", "worker:0"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.processed", 1, "", []string{"exporter_ip:127.0.0.1", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.flowsets", 2, "", []string{"exporter_ip:127.0.0.1", "type:data_flow_set", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.bytes", 120, "", []string{fmt.Sprintf("listener_port:%d", port), "exporter_ip:127.0.0.1", "collector_type:netflow5"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.packets", 1, "", []string{fmt.Sprintf("listener_port:%d", port), "exporter_ip:127.0.0.1", "collector_type:netflow5"})

	flowState.Shutdown()
	aggregator.Stop()

	<-stoppedFlushLoop
	<-stoppedRun
}

func TestFlowAggregator_flush_submitCollectorMetrics_error(t *testing.T) {
	// 1/ Arrange
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	l, err := ddlog.LoggerFromWriterWithMinLevelAndLvlFuncMsgFormat(w, ddlog.DebugLvl)
	require.NoError(t, err)
	ddlog.SetupLogger(l, "debug")

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return nil, errors.New("some prometheus gatherer error")
	})

	// 2/ Act
	aggregator.flush(common.FlushContext{FlushTime: aggregator.TimeNowFunction()})

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)
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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return nil, errors.New("some prometheus gatherer error")
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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)

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
			Integration:      "netflow",
			CollectTimestamp: now.Unix(),
			NetflowExporters: exporters,
		}
		payloadBytes, err := json.Marshal(payload)
		require.NoError(t, err)

		m := message.NewMessage(payloadBytes, nil, "", 0)
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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)

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
	metadataEvent := []byte(`
{
  "namespace":"my-ns",
  "integration": "netflow",
  "netflow_exporters":[
    {
      "id": "my-ns:127.0.0.10:netflow9",
      "ip_address":"127.0.0.10",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`)
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)

	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)

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
	metadataEvent := []byte(`
{
  "namespace":"my-ns1",
  "integration": "netflow",
  "netflow_exporters":[
    {
      "id": "my-ns1:127.0.0.11:netflow9",
      "ip_address":"127.0.0.11",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`)
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)

	// language=json
	metadataEvent2 := []byte(`
{
  "namespace":"my-ns2",
  "integration": "netflow",
  "netflow_exporters":[
    {
      "id": "my-ns2:127.0.0.12:netflow9",
      "ip_address":"127.0.0.12",
      "flow_type":"netflow9"
    }
  ],
  "collect_timestamp": 1681295467
}
`)
	compactMetadataEvent2 := new(bytes.Buffer)
	err = json.Compact(compactMetadataEvent2, metadataEvent2)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent2.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)

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
	epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

	aggregator := NewFlowAggregator(sender, epForwarder, &conf, "my-hostname", logger, rdnsQuerier)

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
	metadataEvent := []byte(`
{
  "namespace":"my-ns1",
  "integration": "netflow",
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
`)
	compactMetadataEvent := new(bytes.Buffer)
	err := json.Compact(compactMetadataEvent, metadataEvent)
	assert.NoError(t, err)
	epForwarder.EXPECT().SendEventPlatformEventBlocking(message.NewMessage(compactMetadataEvent.Bytes(), nil, "", 0), "network-devices-metadata").Return(nil).Times(1)

	// call sendExporterMetadata does not trigger any call to epForwarder.SendEventPlatformEventBlocking(...)
	aggregator.sendExporterMetadata(flows, now)
}

func TestFlowAggregator_getSequenceDelta(t *testing.T) {
	logger := logmock.New(t)
	rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())
	type round struct {
		flowsToFlush          []*common.Flow
		expectedSequenceDelta map[sequenceDeltaKey]sequenceDeltaValue
	}
	tests := []struct {
		name   string
		rounds []round
	}{
		{
			name: "multiple namespaces",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10,
							FlowType:     common.TypeNetFlow5,
						},
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  20,
							FlowType:     common.TypeNetFlow5,
						},
						{
							Namespace:    "ns2",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  30,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 20, Delta: 0},
						{FlowType: common.TypeNetFlow5, Namespace: "ns2", ExporterIP: "127.0.0.11"}: {LastSequence: 30, Delta: 0},
					},
				},
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  30,
							FlowType:     common.TypeNetFlow5,
						},
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  40,
							FlowType:     common.TypeNetFlow5,
						},
						{
							Namespace:    "ns2",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  60,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 40, Delta: 20},
						{FlowType: common.TypeNetFlow5, Namespace: "ns2", ExporterIP: "127.0.0.11"}: {LastSequence: 60, Delta: 30},
					},
				},
			},
		},
		{
			name: "sequence reset",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10000,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 10000, Delta: 0},
					},
				},
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  100,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 100, Delta: 100, Reset: true},
					},
				},
			},
		},
		{
			name: "negative delta and sequence reset for netflow5",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10000,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 10000, Delta: 0},
					},
				},
				{ // trigger sequence reset since delta -1100 is less than -1000
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  8900,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 8900, Delta: 8900, Reset: true},
					},
				},
				{ // negative delta without sequence reset
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  8500,
							FlowType:     common.TypeNetFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 8500, Delta: 0},
					},
				},
			},
		},
		{
			name: "negative delta and sequence reset for sflow5",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10000,
							FlowType:     common.TypeSFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeSFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 10000, Delta: 0},
					},
				},
				{ // trigger sequence reset since delta -1100 is less than -1000
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  8900,
							FlowType:     common.TypeSFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeSFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 8900, Delta: 8900, Reset: true},
					},
				},
				{ // negative delta without sequence reset
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  8500,
							FlowType:     common.TypeSFlow5,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeSFlow5, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 8500, Delta: 0},
					},
				},
			},
		},
		{
			name: "negative delta and sequence reset for netflow9",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10000,
							FlowType:     common.TypeNetFlow9,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow9, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 10000, Delta: 0},
					},
				},
				{ // trigger sequence reset since delta -200 is less than -100
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  9800,
							FlowType:     common.TypeNetFlow9,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow9, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 9800, Delta: 9800, Reset: true},
					},
				},
				{ // negative delta without sequence reset
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  9750,
							FlowType:     common.TypeNetFlow9,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeNetFlow9, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 9750, Delta: 0},
					},
				},
			},
		},
		{
			name: "negative delta and sequence reset for IPFIX",
			rounds: []round{
				{
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  10000,
							FlowType:     common.TypeIPFIX,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeIPFIX, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 10000, Delta: 0},
					},
				},
				{ // trigger sequence reset since delta -200 is less than -100
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  9800,
							FlowType:     common.TypeIPFIX,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeIPFIX, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 9800, Delta: 9800, Reset: true},
					},
				},
				{ // negative delta without sequence reset
					flowsToFlush: []*common.Flow{
						{
							Namespace:    "ns1",
							ExporterAddr: []byte{127, 0, 0, 11},
							SequenceNum:  9750,
							FlowType:     common.TypeIPFIX,
						},
					},
					expectedSequenceDelta: map[sequenceDeltaKey]sequenceDeltaValue{
						{FlowType: common.TypeIPFIX, Namespace: "ns1", ExporterIP: "127.0.0.11"}: {LastSequence: 9750, Delta: 0},
					},
				},
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			sender := mocksender.NewMockSender("")
			conf := config.NetflowConfig{
				StopTimeout:                            10,
				AggregatorBufferSize:                   20,
				AggregatorFlushInterval:                1,
				AggregatorPortRollupThreshold:          10,
				AggregatorRollupTrackerRefreshInterval: 3600,
			}
			agg := NewFlowAggregator(sender, nil, &conf, "my-hostname", logger, rdnsQuerier)
			for roundNum, testRound := range tt.rounds {
				assert.Equal(t, testRound.expectedSequenceDelta, agg.getSequenceDelta(testRound.flowsToFlush), fmt.Sprintf("Test Round %d", roundNum))
			}
		})
	}
}

func TestAggregatorFlushing(t *testing.T) {
	t.Run("it respects FlowCollectionDuration when rescheduling flows", func(t *testing.T) {
		// This test verifies that the aggregator correctly passes the flush config to the flow scheduler
		// by checking that flows are rescheduled with the correct interval after being flushed.
		//
		// Context: The bug that prompted this test was in aggregator.go:98-100 where the ImmediateFlowScheduler
		// was created without passing the flushConfig. This caused RefreshFlushTime() to use a zero-valued
		// FlowCollectionDuration, breaking the flow scheduling logic.
		//
		// This is a behavior-driven test that verifies:
		// 1. A flow is flushed immediately on first occurrence
		// 2. When the same flow arrives again, it's not flushed until FlowCollectionDuration has elapsed
		// 3. After FlowCollectionDuration has elapsed, the flow is flushed correctly

		flushTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:00Z")
		sender := mocksender.NewMockSender("")
		sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("Commit").Return()

		conf := config.NetflowConfig{
			StopTimeout:                            10,
			AggregatorBufferSize:                   20,
			AggregatorFlushInterval:                2, // 2 seconds FlowCollectionDuration
			AggregatorPortRollupThreshold:          10,
			AggregatorRollupTrackerRefreshInterval: 3600,
			AggregatorMaxFlowsPerPeriod:            0, // Use ImmediateFlowScheduler
		}

		ctrl := gomock.NewController(t)
		epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
		epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		logger := logmock.New(t)
		rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

		aggregator := NewFlowAggregator(sender, epForwarder, &conf, "test-hostname", logger, rdnsQuerier)
		aggregator.TimeNowFunction = func() time.Time {
			return flushTime
		}

		// Create a flow that will be sent multiple times
		flow := &common.Flow{
			Namespace:      "test-ns",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 1},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          100,
			Packets:        10,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			EtherType:      uint32(0x0800),
		}

		// First flush: Add flow and flush immediately
		setMockTimeNow(flushTime)
		aggregator.flowAcc.add(flow)

		flushCtx1 := common.FlushContext{
			FlushTime:     flushTime,
			LastFlushedAt: time.Time{},
			NumFlushes:    1,
		}
		flushedCount := aggregator.flush(flushCtx1)
		assert.Equal(t, 1, flushedCount, "First flush should return 1 flow")

		// Second flush: Add the same flow again and attempt to flush before FlowCollectionDuration
		// The flow should NOT be flushed yet because it's scheduled for later
		earlyFlushTime := flushTime.Add(1 * time.Second) // Only 1 second passed, but FlowCollectionDuration is 2 seconds
		setMockTimeNow(earlyFlushTime)

		flow2 := *flow // Copy the flow
		flow2.Bytes = 200
		flow2.Packets = 20
		aggregator.flowAcc.add(&flow2)

		flushCtx2 := common.FlushContext{
			FlushTime:     earlyFlushTime,
			LastFlushedAt: flushTime,
			NumFlushes:    1,
		}
		flushedCount = aggregator.flush(flushCtx2)
		assert.Equal(t, 0, flushedCount, "Second flush should return 0 flows because FlowCollectionDuration hasn't elapsed yet")

		// Third flush: Flush after FlowCollectionDuration has passed
		// Now the flow should be flushed
		correctFlushTime := flushTime.Add(2 * time.Second) // FlowCollectionDuration = 2 seconds
		setMockTimeNow(correctFlushTime)

		flushCtx3 := common.FlushContext{
			FlushTime:     correctFlushTime,
			LastFlushedAt: earlyFlushTime,
			NumFlushes:    1,
		}
		flushedCount = aggregator.flush(flushCtx3)
		assert.Equal(t, 1, flushedCount, "Third flush should return 1 flow after FlowCollectionDuration has elapsed")
	})

	t.Run("it respects FlowCollectionDuration when using TopN/JitterFlowScheduler", func(t *testing.T) {
		// This test verifies that when Top-N is enabled, flushConfig is properly passed to JitterFlowScheduler.
		//
		// Test approach:
		// 1. Add a flow and tick through flushes until it gets flushed
		// 2. Record the flush time (t_flush)
		// 3. Add another flow with the same key
		// 4. Verify it's NOT ready before t_flush + FlowCollectionDuration
		// 5. Verify it IS ready at t_flush + FlowCollectionDuration
		//
		// This directly tests RefreshFlushTime behavior: after flushing, flows should be
		// rescheduled for nextFlush + FlowCollectionDuration (with NO jitter).

		startTime, _ := time.Parse(time.RFC3339, "2019-02-18T16:00:00Z")
		sender := mocksender.NewMockSender("")
		sender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
		sender.On("Commit").Return()

		conf := config.NetflowConfig{
			StopTimeout:                            10,
			AggregatorBufferSize:                   20,
			AggregatorFlushInterval:                30, // 30 seconds FlowCollectionDuration
			AggregatorPortRollupThreshold:          10,
			AggregatorRollupTrackerRefreshInterval: 3600,
			AggregatorMaxFlowsPerPeriod:            100, // High limit so TopN doesn't interfere
		}

		ctrl := gomock.NewController(t)
		epForwarder := eventplatformimpl.NewMockEventPlatformForwarder(ctrl)
		epForwarder.EXPECT().SendEventPlatformEventBlocking(gomock.Any(), gomock.Any()).Return(nil).AnyTimes()

		logger := logmock.New(t)
		rdnsQuerier := fxutil.Test[rdnsquerier.Component](t, rdnsquerierfxmock.MockModule())

		aggregator := NewFlowAggregator(sender, epForwarder, &conf, "test-hostname", logger, rdnsQuerier)
		aggregator.TimeNowFunction = func() time.Time {
			return startTime
		}

		// Step 1: Create a flow that will be aggregated
		flow := &common.Flow{
			Namespace:      "test-ns",
			FlowType:       common.TypeNetFlow9,
			ExporterAddr:   []byte{127, 0, 0, 1},
			StartTimestamp: 1234568,
			EndTimestamp:   1234569,
			Bytes:          100,
			Packets:        10,
			SrcAddr:        []byte{10, 10, 10, 10},
			DstAddr:        []byte{10, 10, 10, 20},
			IPProtocol:     uint32(6),
			SrcPort:        2000,
			DstPort:        80,
			EtherType:      uint32(0x0800),
		}

		setMockTimeNow(startTime)
		aggregator.flowAcc.add(flow)

		// Step 2: Tick through flushes until the flow is flushed
		// JitterFlowScheduler schedules with random jitter [0, FlowCollectionDuration)
		// So we need to tick up to the full FlowCollectionDuration to guarantee it's flushed
		var actualFlushTime time.Time
		flushInterval := 10 * time.Second // FlushTickFrequency

		for i := 0; i < 4; i++ { // Tick 4 times (0s, 10s, 20s, 30s)
			currentTime := startTime.Add(time.Duration(i) * flushInterval)
			setMockTimeNow(currentTime)

			flushCtx := common.FlushContext{
				FlushTime:     currentTime,
				LastFlushedAt: startTime.Add(time.Duration(i-1) * flushInterval),
				NumFlushes:    1,
			}

			if i == 0 {
				flushCtx.LastFlushedAt = time.Time{}
			}

			flushedCount := aggregator.flush(flushCtx)
			if flushedCount > 0 {
				actualFlushTime = currentTime
				assert.Equal(t, 1, flushedCount, "Should flush exactly 1 flow")
				break
			}
		}

		assert.False(t, actualFlushTime.IsZero(), "Flow should have been flushed within FlowCollectionDuration")

		// Step 3: Add another flow with the same key (will be aggregated with the first)
		flow2 := *flow
		flow2.Bytes = 200
		flow2.Packets = 20

		aggregator.flowAcc.add(&flow2)

		// Step 4: Verify flow is NOT ready before actualFlushTime + FlowCollectionDuration
		// This is the critical test: RefreshFlushTime should add FlowCollectionDuration.
		// It should not flush at t + 10s nor t + 20s
		tick1 := actualFlushTime.Add(10 * time.Second)
		setMockTimeNow(tick1)
		flushCtx := common.FlushContext{
			FlushTime:     tick1,
			LastFlushedAt: actualFlushTime,
			NumFlushes:    1,
		}
		flushedCount := aggregator.flush(flushCtx)
		assert.Equal(t, 0, flushedCount, "Flow should NOT be ready before actualFlushTime + FlowCollectionDuration")

		tick2 := actualFlushTime.Add(20 * time.Second)
		setMockTimeNow(tick2)
		flushCtx = common.FlushContext{
			FlushTime:     tick2,
			LastFlushedAt: actualFlushTime.Add(10 * time.Second),
			NumFlushes:    1,
		}
		flushedCount = aggregator.flush(flushCtx)
		assert.Equal(t, 0, flushedCount, "Flow should NOT be ready before actualFlushTime + FlowCollectionDuration")

		// Step 5: Verify flow IS ready at actualFlushTime + FlowCollectionDuration
		tick3 := actualFlushTime.Add(30 * time.Second) // Full FlowCollectionDuration
		setMockTimeNow(tick3)
		flushCtx = common.FlushContext{
			FlushTime:     tick3,
			LastFlushedAt: actualFlushTime.Add(20 * time.Second),
			NumFlushes:    1,
		}
		flushedCount = aggregator.flush(flushCtx)
		assert.Equal(t, 1, flushedCount, "Flow should be ready at actualFlushTime + FlowCollectionDuration")

		// Verify TopN metrics were submitted
		sender.AssertCalled(t, "Histogram", "datadog.netflow.flow_truncation.runtime_ms", mock.Anything, mock.Anything, mock.Anything)
		sender.AssertCalled(t, "Gauge", "datadog.netflow.flow_truncation.threshold_value", float64(100), mock.Anything, mock.Anything)
	})
}
