package flowaggregator

import (
	"bytes"
	"encoding/json"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	coreconfig "github.com/DataDog/datadog-agent/pkg/config"
	"github.com/DataDog/datadog-agent/pkg/netflow/common"
	"github.com/DataDog/datadog-agent/pkg/netflow/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"net"
	"sync"
	"testing"
	"time"
)

func TestAggregator(t *testing.T) {
	stoppedMu := sync.RWMutex{} // Mutex needed to avoid race condition in test

	coreconfig.Datadog.Set("hostname", "my-hostname")
	sender := mocksender.NewMockSender("")
	sender.On("Count", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	sender.On("EventPlatformEvent", mock.Anything, mock.Anything).Return()
	sender.On("Commit").Return()
	conf := config.NetflowConfig{
		StopTimeout:             10,
		AggregatorBufferSize:    20,
		AggregatorFlushInterval: 1,
		LogPayloads:             true,
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
		ExporterAddr:   net.IP([]byte{127, 0, 0, 1}).String(),
		StartTimestamp: 1234568,
		EndTimestamp:   1234569,
		Bytes:          20,
		Packets:        4,
		SrcAddr:        net.IP([]byte{10, 10, 10, 10}).String(),
		DstAddr:        net.IP([]byte{10, 10, 10, 20}).String(),
		IPProtocol:     uint32(6),
		SrcPort:        uint32(2000),
		DstPort:        uint32(80),
	}

	aggregator := NewFlowAggregator(sender, &conf)
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

	time.Sleep(3 * time.Second)

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
  "ether_type": "0",
  "ip_protocol": "6",
  "exporter": {
    "ip": "127.0.0.1"
  },
  "source": {
    "ip": "10.10.10.10",
    "port": 2000,
    "mac": "00:00:00:00:00:00",
    "mask": "0.0.0.0/24"
  },
  "destination": {
    "ip": "10.10.10.20",
    "port": 80,
    "mac": "",
    "mask": ""
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
  "namespace": "my-ns",
  "host": "my-hostname",
  "tcp_flags": [
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
	sender.AssertEventPlatformEvent(t, compactEvent.String(), "network-devices-netflow")
	sender.AssertMetric(t, "Count", "datadog.newflow.aggregator.flows_flushed", 1, "", []string{"exporter:127.0.0.1", "flow_type:netflow9"})

	// Test aggregator Stop
	assert.False(t, expectStartExisted)
	aggregator.Stop()
	time.Sleep(500 * time.Millisecond)
	stoppedMu.Lock()
	assert.True(t, expectStartExisted)
	stoppedMu.Unlock()
}
