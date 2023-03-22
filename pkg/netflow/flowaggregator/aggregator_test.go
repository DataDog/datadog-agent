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
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cihub/seelog"
	"github.com/gogo/protobuf/proto"
	"github.com/prometheus/client_golang/prometheus"
	promClient "github.com/prometheus/client_model/go"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/log"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/goflowlib"
)

func waitForFlowsToBeFlushed(aggregator *FlowAggregator, timeoutDuration time.Duration, minEvents uint64) error {
	timeout := time.After(timeoutDuration)
	tick := time.Tick(500 * time.Millisecond)
	// Keep trying until we're timed out or got a result or got an error
	for {
		select {
		// Got a timeout! fail with a timeout error
		case <-timeout:
			return fmt.Errorf("timeout error waiting for events")
		// Got a tick, we should check on doSomething()
		case <-tick:
			if aggregator.flushedFlowCount.Load() >= minEvents {
				return nil
			}
		}
	}
}

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
		DeviceAddr:     []byte{127, 0, 0, 1},
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

	aggregator := NewFlowAggregator(sender, &conf, "my-hostname")
	aggregator.flushFlowsToSendInterval = 1 * time.Second
	inChan := aggregator.GetFlowInChan()

	expectStartExisted := false
	go func() {
		aggregator.Start()
		stoppedMu.Lock()
		expectStartExisted = true
		stoppedMu.Unlock()
	}()
	inChan <- flow

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
    "ip": "127.0.0.1",
    "namespace": "my-ns"
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

	err = waitForFlowsToBeFlushed(aggregator, 10*time.Second, 1)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-netflow")
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
				Port:     uint16(port),
				Workers:  10,
			},
		},
	}

	aggregator := NewFlowAggregator(sender, &conf, "my-hostname")
	aggregator.flushFlowsToSendInterval = 1 * time.Second

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
	err = goflowlib.SendUDPPacket(port, goflowlib.MockNetflowV5Data)
	require.NoError(t, err, "error sending udp packet")

	// language=json
	event := []byte(`
{
    "type": "netflow5",
    "sampling_rate": 0,
    "direction": "ingress",
    "start": 1540209169,
    "end": 1540209169,
    "bytes": 590,
    "packets": 5,
    "ether_type": "IPv4",
    "ip_protocol": "TCP",
    "device": {
        "ip": "127.0.0.1",
        "namespace": "default"
    },
    "source": {
        "ip": "10.128.2.121",
        "port": "8080",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "destination": {
        "ip": "10.128.2.1",
        "port": "47384",
        "mac": "00:00:00:00:00:00",
        "mask": "0.0.0.0/0"
    },
    "ingress": {
        "interface": {
            "index": 9
        }
    },
    "egress": {
        "interface": {
            "index": 2
        }
    },
    "host": "my-hostname",
    "tcp_flags": [
        "FIN",
        "SYN",
        "PSH",
        "ACK"
    ],
    "next_hop": {
        "ip": "0.0.0.0"
    }
}
`)
	compactEvent := new(bytes.Buffer)
	err = json.Compact(compactEvent, event)
	assert.NoError(t, err)

	err = waitForFlowsToBeFlushed(aggregator, 3*time.Second, 1)
	assert.NoError(t, err)

	sender.AssertEventPlatformEvent(t, compactEvent.Bytes(), "network-devices-netflow")
	sender.AssertMetric(t, "Count", "datadog.netflow.aggregator.flows_flushed", 6, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.aggregator.flows_received", 6, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.flows_contexts", 6, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.current_store_size", 10, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.port_rollup.new_store_size", 10, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.capacity", 20, "", nil)
	sender.AssertMetric(t, "Gauge", "datadog.netflow.aggregator.input_buffer.length", 0, "", nil)
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.decoder.messages", 1, "", []string{"collector_type:netflow5", "worker:0"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.flows", 1, "", []string{"device_ip:127.0.0.1", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.processor.flowsets", 6, "", []string{"device_ip:127.0.0.1", "type:data_flow_set", "version:5", "flow_protocol:netflow"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.bytes", 312, "", []string{"listener_port:52056", "device_ip:127.0.0.1", "collector_type:netflow5"})
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.traffic.packets", 1, "", []string{"listener_port:52056", "device_ip:127.0.0.1", "collector_type:netflow5"})

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
	aggregator := NewFlowAggregator(sender, &conf, "my-hostname")
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
	aggregator := NewFlowAggregator(sender, &conf, "my-hostname")
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
	aggregator := NewFlowAggregator(sender, &conf, "my-hostname")
	aggregator.goflowPrometheusGatherer = prometheus.GathererFunc(func() ([]*promClient.MetricFamily, error) {
		return nil, fmt.Errorf("some prometheus gatherer error")
	})

	// 2/ Act
	err := aggregator.submitCollectorMetrics()

	// 3/ Assert
	assert.EqualError(t, err, "some prometheus gatherer error")
}
