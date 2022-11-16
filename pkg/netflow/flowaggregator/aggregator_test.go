// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2022-present Datadog, Inc.

package flowaggregator

import (
	"bytes"
	"encoding/json"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
)

func TestAggregator1(t *testing.T) {
	runTest(t)
}

func TestAggregator2(t *testing.T) {
	runTest(t)
}

func TestAggregator3(t *testing.T) {
	runTest(t)
}

func TestAggregator4(t *testing.T) {
	runTest(t)
}

func TestAggregator5(t *testing.T) {
	runTest(t)
}

func TestAggregator6(t *testing.T) {
	runTest(t)
}

func TestAggregator7(t *testing.T) {
	runTest(t)
}

func TestAggregator8(t *testing.T) {
	runTest(t)
}

func TestAggregator9(t *testing.T) {
	runTest(t)
}

func TestAggregator10(t *testing.T) {
	runTest(t)
}

func TestAggregator11(t *testing.T) {
	runTest(t)
}

func TestAggregator12(t *testing.T) {
	runTest(t)
}

func TestAggregator13(t *testing.T) {
	runTest(t)
}

func TestAggregator14(t *testing.T) {
	runTest(t)
}

func TestAggregator15(t *testing.T) {
	runTest(t)
}

func TestAggregator16(t *testing.T) {
	runTest(t)
}

func TestAggregator17(t *testing.T) {
	runTest(t)
}

func TestAggregator18(t *testing.T) {
	runTest(t)
}

func TestAggregator19(t *testing.T) {
	runTest(t)
}

func TestAggregator20(t *testing.T) {
	runTest(t)
}

func TestAggregator21(t *testing.T) {
	runTest(t)
}

func TestAggregator22(t *testing.T) {
	runTest(t)
}

func TestAggregator23(t *testing.T) {
	runTest(t)
}

func TestAggregator24(t *testing.T) {
	runTest(t)
}

func TestAggregator25(t *testing.T) {
	runTest(t)
}

func TestAggregator26(t *testing.T) {
	runTest(t)
}

func TestAggregator27(t *testing.T) {
	runTest(t)
}

func TestAggregator28(t *testing.T) {
	runTest(t)
}

func TestAggregator29(t *testing.T) {
	runTest(t)
}

func TestAggregator30(t *testing.T) {
	runTest(t)
}

func TestAggregator31(t *testing.T) {
	runTest(t)
}

func TestAggregator32(t *testing.T) {
	runTest(t)
}

func TestAggregator33(t *testing.T) {
	runTest(t)
}

func TestAggregator34(t *testing.T) {
	runTest(t)
}

func TestAggregator35(t *testing.T) {
	runTest(t)
}

func TestAggregator36(t *testing.T) {
	runTest(t)
}

func TestAggregator37(t *testing.T) {
	runTest(t)
}

func TestAggregator38(t *testing.T) {
	runTest(t)
}

func TestAggregator39(t *testing.T) {
	runTest(t)
}

func TestAggregator40(t *testing.T) {
	runTest(t)
}

func TestAggregator41(t *testing.T) {
	runTest(t)
}

func TestAggregator42(t *testing.T) {
	runTest(t)
}

func TestAggregator43(t *testing.T) {
	runTest(t)
}

func TestAggregator44(t *testing.T) {
	runTest(t)
}

func TestAggregator45(t *testing.T) {
	runTest(t)
}

func TestAggregator46(t *testing.T) {
	runTest(t)
}

func TestAggregator47(t *testing.T) {
	runTest(t)
}

func TestAggregator48(t *testing.T) {
	runTest(t)
}

func TestAggregator49(t *testing.T) {
	runTest(t)
}

func TestAggregator50(t *testing.T) {
	runTest(t)
}

func TestAggregator51(t *testing.T) {
	runTest(t)
}

func TestAggregator52(t *testing.T) {
	runTest(t)
}

func TestAggregator53(t *testing.T) {
	runTest(t)
}

func TestAggregator54(t *testing.T) {
	runTest(t)
}

func TestAggregator55(t *testing.T) {
	runTest(t)
}

func TestAggregator56(t *testing.T) {
	runTest(t)
}

func TestAggregator57(t *testing.T) {
	runTest(t)
}

func TestAggregator58(t *testing.T) {
	runTest(t)
}

func TestAggregator59(t *testing.T) {
	runTest(t)
}

func TestAggregator60(t *testing.T) {
	runTest(t)
}

func TestAggregator61(t *testing.T) {
	runTest(t)
}

func TestAggregator62(t *testing.T) {
	runTest(t)
}

func TestAggregator63(t *testing.T) {
	runTest(t)
}

func TestAggregator64(t *testing.T) {
	runTest(t)
}

func TestAggregator65(t *testing.T) {
	runTest(t)
}

func TestAggregator66(t *testing.T) {
	runTest(t)
}

func TestAggregator67(t *testing.T) {
	runTest(t)
}

func TestAggregator68(t *testing.T) {
	runTest(t)
}

func TestAggregator69(t *testing.T) {
	runTest(t)
}

func TestAggregator70(t *testing.T) {
	runTest(t)
}

func TestAggregator71(t *testing.T) {
	runTest(t)
}

func TestAggregator72(t *testing.T) {
	runTest(t)
}

func TestAggregator73(t *testing.T) {
	runTest(t)
}

func TestAggregator74(t *testing.T) {
	runTest(t)
}

func TestAggregator75(t *testing.T) {
	runTest(t)
}

func TestAggregator76(t *testing.T) {
	runTest(t)
}

func TestAggregator77(t *testing.T) {
	runTest(t)
}

func TestAggregator78(t *testing.T) {
	runTest(t)
}

func TestAggregator79(t *testing.T) {
	runTest(t)
}

func TestAggregator80(t *testing.T) {
	runTest(t)
}

func TestAggregator81(t *testing.T) {
	runTest(t)
}

func TestAggregator82(t *testing.T) {
	runTest(t)
}

func TestAggregator83(t *testing.T) {
	runTest(t)
}

func TestAggregator84(t *testing.T) {
	runTest(t)
}

func TestAggregator85(t *testing.T) {
	runTest(t)
}

func TestAggregator86(t *testing.T) {
	runTest(t)
}

func TestAggregator87(t *testing.T) {
	runTest(t)
}

func TestAggregator88(t *testing.T) {
	runTest(t)
}

func TestAggregator89(t *testing.T) {
	runTest(t)
}

func TestAggregator90(t *testing.T) {
	runTest(t)
}

func TestAggregator91(t *testing.T) {
	runTest(t)
}

func TestAggregator92(t *testing.T) {
	runTest(t)
}

func TestAggregator93(t *testing.T) {
	runTest(t)
}

func TestAggregator94(t *testing.T) {
	runTest(t)
}

func TestAggregator95(t *testing.T) {
	runTest(t)
}

func TestAggregator96(t *testing.T) {
	runTest(t)
}

func TestAggregator97(t *testing.T) {
	runTest(t)
}

func TestAggregator98(t *testing.T) {
	runTest(t)
}

func TestAggregator99(t *testing.T) {
	runTest(t)
}

func TestAggregator100(t *testing.T) {
	runTest(t)
}

func runTest(t *testing.T) {
	stoppedMu := sync.RWMutex{} // Mutex needed to avoid race condition in test

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
	aggregator.flushInterval = 1 * time.Second
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

	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-netflow")
	sender.AssertMetric(t, "MonotonicCount", "datadog.netflow.aggregator.flows_flushed", 1, "", nil)
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
