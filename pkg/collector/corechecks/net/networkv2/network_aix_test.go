// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build aix

package networkv2

import (
	"errors"
	"testing"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
)

type fakeNetworkStats struct {
	counterStats             []net.IOCountersStat
	counterStatsError        error
	protoCountersStats       []net.ProtoCountersStat
	protoCountersStatsError  error
	connectionStatsUDP4      []net.ConnectionStat
	connectionStatsUDP4Error error
	connectionStatsUDP6      []net.ConnectionStat
	connectionStatsUDP6Error error
	connectionStatsTCP4      []net.ConnectionStat
	connectionStatsTCP4Error error
	connectionStatsTCP6      []net.ConnectionStat
	connectionStatsTCP6Error error
	netstatTCPExtCounters    map[string]int64
	netstatTCPExtCountersErr error
}

// IOCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) IOCounters(_ bool) ([]net.IOCountersStat, error) {
	return n.counterStats, n.counterStatsError
}

// ProtoCounters returns the inner values of protoCountersStats and protoCountersStatsError
func (n *fakeNetworkStats) ProtoCounters(_ []string) ([]net.ProtoCountersStat, error) {
	return n.protoCountersStats, n.protoCountersStatsError
}

// Connections returns the inner values of the connectionStats* fields matching kind
func (n *fakeNetworkStats) Connections(kind string) ([]net.ConnectionStat, error) {
	switch kind {
	case "udp4":
		return n.connectionStatsUDP4, n.connectionStatsUDP4Error
	case "udp6":
		return n.connectionStatsUDP6, n.connectionStatsUDP6Error
	case "tcp4":
		return n.connectionStatsTCP4, n.connectionStatsTCP4Error
	case "tcp6":
		return n.connectionStatsTCP6, n.connectionStatsTCP6Error
	}
	return nil, nil
}

// NetstatTCPExtCounters returns the inner values of netstatTCPExtCounters and netstatTCPExtCountersErr
func (n *fakeNetworkStats) NetstatTCPExtCounters() (map[string]int64, error) {
	return n.netstatTCPExtCounters, n.netstatTCPExtCountersErr
}

func createTestNetworkCheck(mockNetStats networkStats) *NetworkCheck {
	return &NetworkCheck{
		net: mockNetStats,
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics: true,
			},
		},
	}
}

func TestDefaultConfiguration(t *testing.T) {
	check := createTestNetworkCheck(nil)
	err := check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test", "provider")

	assert.Nil(t, err)
	assert.Equal(t, false, check.config.instance.CollectConnectionState)
	assert.Equal(t, []string(nil), check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "", check.config.instance.ExcludedInterfaceRe)
}

func TestConfiguration(t *testing.T) {
	check := createTestNetworkCheck(nil)
	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
excluded_interfaces:
    - en0
    - lo0
excluded_interface_re: "en.*"
`)
	err := check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test", "provider")

	assert.Nil(t, err)
	assert.Equal(t, true, check.config.instance.CollectConnectionState)
	assert.Equal(t, true, check.config.instance.CollectCountMetrics)
	assert.ElementsMatch(t, []string{"en0", "lo0"}, check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "en.*", check.config.instance.ExcludedInterfaceRe)
	assert.NotNil(t, check.config.instance.ExcludedInterfacePattern)
}

func TestNetworkCheck(t *testing.T) {
	netStats := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "en0",
				BytesRecv:   10,
				BytesSent:   11,
				PacketsRecv: 12,
				Dropin:      13,
				Errin:       14,
				PacketsSent: 15,
				Dropout:     16,
				Errout:      17,
			},
			{
				Name:        "lo0",
				BytesRecv:   18,
				BytesSent:   19,
				PacketsRecv: 20,
				Dropin:      21,
				Errin:       22,
				PacketsSent: 23,
				Dropout:     24,
				Errout:      25,
			},
		},
		protoCountersStats: []net.ProtoCountersStat{
			{
				Protocol: "tcp",
				Stats: map[string]int64{
					"RetransSegs":     1,
					"InSegs":          2,
					"OutSegs":         3,
					"ListenOverflows": 4,
					"ListenDrops":     5,
					"TCPBacklogDrop":  6,
					"TCPRetransFail":  7,
				},
			},
			{
				Protocol: "udp",
				Stats: map[string]int64{
					"InDatagrams":  8,
					"NoPorts":      9,
					"InErrors":     10,
					"OutDatagrams": 11,
					"RcvbufErrors": 12,
					"SndbufErrors": 13,
					"InCsumErrors": 14,
				},
			},
		},
		// gopsutil's AIX backend never populates TCPExt counters (no /proc/net/netstat);
		// exercise that no-op path explicitly.
		netstatTCPExtCounters: nil,
		connectionStatsUDP4: []net.ConnectionStat{
			// AIX netstat output omits the state column for UDP rows, so gopsutil leaves
			// Status as "" here.
			{Status: ""},
		},
		connectionStatsUDP6: []net.ConnectionStat{
			{Status: ""},
			{Status: ""},
		},
		connectionStatsTCP4: []net.ConnectionStat{
			{Status: "ESTABLISHED"},
			{Status: "SYN_SENT"},
			{Status: "SYN_RECV"},
			{Status: "FIN_WAIT1"},
			{Status: "FIN_WAIT2"},
			{Status: "TIME_WAIT"},
			{Status: "CLOSE"},
			{Status: "CLOSE_WAIT"},
			{Status: "LAST_ACK"},
			{Status: "LISTEN"},
			{Status: "CLOSING"},
		},
		connectionStatsTCP6: []net.ConnectionStat{
			{Status: "ESTABLISHED"},
			{Status: "LISTEN"},
		},
	}

	networkCheck := createTestNetworkCheck(netStats)

	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
`)

	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test", "provider")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	en0Tags := []string{"device:en0", "device_name:en0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", en0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)

	var customTags []string

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_segs", float64(1), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_segs.count", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(2), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_segs.count", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(3), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_segs.count", float64(3), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(4), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.listen_overflows.count", float64(4), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_drops", float64(5), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.listen_drops.count", float64(5), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(6), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.backlog_drops.count", float64(6), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(7), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.failed_retransmits.count", float64(7), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_datagrams", float64(8), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_datagrams.count", float64(8), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.no_ports", float64(9), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.no_ports.count", float64(9), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_errors", float64(10), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_errors.count", float64(10), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.out_datagrams", float64(11), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.out_datagrams.count", float64(11), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.rcv_buf_errors", float64(12), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.rcv_buf_errors.count", float64(12), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.snd_buf_errors", float64(13), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.snd_buf_errors.count", float64(13), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_csum_errors", float64(14), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_csum_errors.count", float64(14), "", customTags)

	// The empty-status UDP rows must still count as "connections" (AIX-specific gopsutil quirk).
	mockSender.AssertCalled(t, "Gauge", "system.net.udp4.connections", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.udp6.connections", float64(2), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.established", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.opening", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.closing", float64(6), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.time_wait", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.listening", float64(1), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.established", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.listening", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.opening", float64(0), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.closing", float64(0), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.time_wait", float64(0), "", customTags)

	mockSender.AssertCalled(t, "Commit")
}

// TestNetworkCheckNetstatTCPExtCountersMerged verifies that TCPExt counters returned by
// NetstatTCPExtCounters (a no-op on real AIX, but exercised here to cover the merge path)
// get merged into the tcp protocol stats before submission.
func TestNetworkCheckNetstatTCPExtCountersMerged(t *testing.T) {
	netStats := &fakeNetworkStats{
		protoCountersStats: []net.ProtoCountersStat{
			{
				Protocol: "tcp",
				Stats: map[string]int64{
					"InSegs": 100,
				},
			},
		},
		netstatTCPExtCounters: map[string]int64{
			"OutSegs": 200,
		},
	}

	networkCheck := createTestNetworkCheck(netStats)
	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test", "provider")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(100), "", []string(nil))
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(200), "", []string(nil))
}

// TestNetworkCheckNetstatTCPExtCountersError verifies that an error from
// NetstatTCPExtCounters is only logged, not propagated by Run.
func TestNetworkCheckNetstatTCPExtCountersError(t *testing.T) {
	netStats := &fakeNetworkStats{
		protoCountersStats: []net.ProtoCountersStat{
			{
				Protocol: "tcp",
				Stats: map[string]int64{
					"InSegs": 1,
				},
			},
		},
		netstatTCPExtCountersErr: errors.New("boom"),
	}

	networkCheck := createTestNetworkCheck(netStats)
	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test", "provider")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(1), "", []string(nil))
}

func TestExcludedInterfaces(t *testing.T) {
	netStats := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{Name: "en0", BytesRecv: 10, BytesSent: 11},
			{Name: "lo0", BytesRecv: 18, BytesSent: 19},
		},
	}

	networkCheck := createTestNetworkCheck(netStats)

	rawInstanceConfig := []byte(`
excluded_interfaces:
    - lo0
`)

	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test", "provider")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	en0Tags := []string{"device:en0", "device_name:en0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", en0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", en0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
}

func TestExcludedInterfacesRe(t *testing.T) {
	netStats := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{Name: "en0", BytesRecv: 10, BytesSent: 11},
			{Name: "en1", BytesRecv: 18, BytesSent: 19},
			{Name: "lo0", BytesRecv: 26, BytesSent: 27},
		},
	}

	networkCheck := createTestNetworkCheck(netStats)

	rawInstanceConfig := []byte(`
excluded_interface_re: "en[0-9]"
`)

	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test", "provider")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	en0Tags := []string{"device:en0", "device_name:en0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", en0Tags)

	en1Tags := []string{"device:en1", "device_name:en1"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", en1Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(26), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(27), "", lo0Tags)
}
