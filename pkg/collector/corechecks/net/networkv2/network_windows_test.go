// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
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
	tcp4Stats                *mibTCPStats
	tcp4StatsError           error
	tcp6Stats                *mibTCPStats
	tcp6StatsError           error
}

// IOCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) IOCounters(_ bool) ([]net.IOCountersStat, error) {
	return n.counterStats, n.counterStatsError
}

// ProtoCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) ProtoCounters(_ []string) ([]net.ProtoCountersStat, error) {
	return n.protoCountersStats, n.protoCountersStatsError
}

// Connections returns the inner values of counterStats and counterStatsError
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

// TCPStats returns the mocked values for the GetTcpStatisticsEx syscall
func (n *fakeNetworkStats) TCPStats(kind string) (*mibTCPStats, error) {
	switch kind {
	case "tcp4":
		return n.tcp4Stats, n.tcp4StatsError
	case "tcp6":
		return n.tcp6Stats, n.tcp6StatsError
	}
	return nil, nil
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
	check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

	assert.Equal(t, false, check.config.instance.CollectConnectionState)
	assert.Equal(t, []string(nil), check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "", check.config.instance.ExcludedInterfaceRe)
}

func TestConfiguration(t *testing.T) {
	check := createTestNetworkCheck(nil)
	rawInstanceConfig := []byte(`
collect_connection_state: true
excluded_interfaces:
    - eth0
    - lo0
excluded_interface_re: "eth.*"
`)
	err := check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	assert.Nil(t, err)
	assert.Equal(t, true, check.config.instance.CollectConnectionState)
	assert.ElementsMatch(t, []string{"eth0", "lo0"}, check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "eth.*", check.config.instance.ExcludedInterfaceRe)
}

func TestNetworkCheck(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
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
		connectionStatsUDP4: []net.ConnectionStat{
			{
				Status: "",
			},
		},
		connectionStatsUDP6: []net.ConnectionStat{
			{
				Status: "",
			},
			{
				Status: "",
			},
		},
		connectionStatsTCP4: []net.ConnectionStat{
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECEIVED",
			},
			{
				Status: "FIN_WAIT_1",
			},
			{
				Status: "FIN_WAIT_2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSED",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
		},

		connectionStatsTCP6: []net.ConnectionStat{
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECEIVED",
			},
			{
				Status: "FIN_WAIT_1",
			},
			{
				Status: "FIN_WAIT_2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSED",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
			{
				Status: "ESTABLISHED",
			},
			{
				Status: "SYN_SENT",
			},
			{
				Status: "SYN_RECEIVED",
			},
			{
				Status: "FIN_WAIT_1",
			},
			{
				Status: "FIN_WAIT_2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSED",
			},
			{
				Status: "CLOSE_WAIT",
			},
			{
				Status: "LAST_ACK",
			},
			{
				Status: "LISTEN",
			},
			{
				Status: "CLOSING",
			},
		},
		tcp4Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(1), // not used
			DwRtoMin:       uint32(2), // not used
			DwRtoMax:       uint32(3), // not used
			DwMaxConn:      uint32(4), // not used
			DwActiveOpens:  uint32(5),
			DwPassiveOpens: uint32(6),
			DwAttemptFails: uint32(7),
			DwEstabResets:  uint32(8),
			DwCurrEstab:    uint32(9),
			DwInSegs:       uint32(10),
			DwOutSegs:      uint32(11),
			DwRetransSegs:  uint32(12),
			DwInErrs:       uint32(13),
			DwOutRsts:      uint32(14),
			DwNumConns:     uint32(15),
		},
		tcp6Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(16), // not used
			DwRtoMin:       uint32(17), // not used
			DwRtoMax:       uint32(18), // not used
			DwMaxConn:      uint32(19), // not used
			DwActiveOpens:  uint32(20),
			DwPassiveOpens: uint32(21),
			DwAttemptFails: uint32(22),
			DwEstabResets:  uint32(23),
			DwCurrEstab:    uint32(24),
			DwInSegs:       uint32(25),
			DwOutSegs:      uint32(26),
			DwRetransSegs:  uint32(27),
			DwInErrs:       uint32(28),
			DwOutRsts:      uint32(29),
			DwNumConns:     uint32(30),
		},
	}

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	var customTags []string

	eth0Tags := []string{"device:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)

	mockSender.AssertCalled(t, "Gauge", "system.net.udp4.connections", float64(1), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.udp6.connections", float64(2), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.established", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.opening", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.time_wait", float64(1), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.closing", float64(6), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.listening", float64(1), "", customTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.established", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.opening", float64(4), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.time_wait", float64(2), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.closing", float64(12), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.listening", float64(2), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.active_opens", float64(25), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.active_opens.count", float64(25), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.passive_opens", float64(27), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.passive_opens.count", float64(27), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.attempt_fails", float64(29), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.attempt_fails.count", float64(29), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.established_resets", float64(31), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.established_resets.count", float64(31), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp.current_established", float64(33), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(35), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_segs.count", float64(35), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(37), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_segs.count", float64(37), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_segs", float64(39), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_segs.count", float64(39), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_errors", float64(41), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_errors.count", float64(41), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_resets", float64(43), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_resets.count", float64(43), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp.connections", float64(45), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.active_opens", float64(5), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.active_opens.count", float64(5), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.passive_opens", float64(6), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.passive_opens.count", float64(6), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.attempt_fails", float64(7), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.attempt_fails.count", float64(7), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.established_resets", float64(8), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.established_resets.count", float64(8), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.current_established", float64(9), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.in_segs", float64(10), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.in_segs.count", float64(10), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.out_segs", float64(11), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.out_segs.count", float64(11), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.retrans_segs", float64(12), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.retrans_segs.count", float64(12), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.in_errors", float64(13), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.in_errors.count", float64(13), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp4.out_resets", float64(14), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp4.out_resets.count", float64(14), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp4.connections", float64(15), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.active_opens", float64(20), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.active_opens.count", float64(20), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.passive_opens", float64(21), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.passive_opens.count", float64(21), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.attempt_fails", float64(22), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.attempt_fails.count", float64(22), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.established_resets", float64(23), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.established_resets.count", float64(23), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.current_established", float64(24), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.in_segs", float64(25), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.in_segs.count", float64(25), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.out_segs", float64(26), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.out_segs.count", float64(26), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.retrans_segs", float64(27), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.retrans_segs.count", float64(27), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.in_errors", float64(28), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.in_errors.count", float64(28), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp6.out_resets", float64(29), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp6.out_resets.count", float64(29), "", customTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.tcp6.connections", float64(30), "", customTags)

	mockSender.AssertCalled(t, "Commit")
}

func TestExcludedInterfaces(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
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
		tcp4Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(1), // not used
			DwRtoMin:       uint32(2), // not used
			DwRtoMax:       uint32(3), // not used
			DwMaxConn:      uint32(4), // not used
			DwActiveOpens:  uint32(5),
			DwPassiveOpens: uint32(6),
			DwAttemptFails: uint32(7),
			DwEstabResets:  uint32(8),
			DwCurrEstab:    uint32(9),
			DwInSegs:       uint32(10),
			DwOutSegs:      uint32(11),
			DwRetransSegs:  uint32(12),
			DwInErrs:       uint32(13),
			DwOutRsts:      uint32(14),
			DwNumConns:     uint32(15),
		},
		tcp6Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(16), // not used
			DwRtoMin:       uint32(17), // not used
			DwRtoMax:       uint32(18), // not used
			DwMaxConn:      uint32(19), // not used
			DwActiveOpens:  uint32(20),
			DwPassiveOpens: uint32(21),
			DwAttemptFails: uint32(22),
			DwEstabResets:  uint32(23),
			DwCurrEstab:    uint32(24),
			DwInSegs:       uint32(25),
			DwOutSegs:      uint32(26),
			DwRetransSegs:  uint32(27),
			DwInErrs:       uint32(28),
			DwOutRsts:      uint32(29),
			DwNumConns:     uint32(30),
		},
	}

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
excluded_interfaces:
    - lo0
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)
}

func TestExcludedInterfacesRe(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
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
				Name:        "eth1",
				BytesRecv:   18,
				BytesSent:   19,
				PacketsRecv: 20,
				Dropin:      21,
				Errin:       22,
				PacketsSent: 23,
				Dropout:     24,
				Errout:      25,
			},
			{
				Name:        "lo0",
				BytesRecv:   26,
				BytesSent:   27,
				PacketsRecv: 28,
				Dropin:      29,
				Errin:       30,
				PacketsSent: 31,
				Dropout:     32,
				Errout:      33,
			},
		},
		tcp4Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(1), // not used
			DwRtoMin:       uint32(2), // not used
			DwRtoMax:       uint32(3), // not used
			DwMaxConn:      uint32(4), // not used
			DwActiveOpens:  uint32(5),
			DwPassiveOpens: uint32(6),
			DwAttemptFails: uint32(7),
			DwEstabResets:  uint32(8),
			DwCurrEstab:    uint32(9),
			DwInSegs:       uint32(10),
			DwOutSegs:      uint32(11),
			DwRetransSegs:  uint32(12),
			DwInErrs:       uint32(13),
			DwOutRsts:      uint32(14),
			DwNumConns:     uint32(15),
		},
		tcp6Stats: &mibTCPStats{
			DwRtoAlgorithm: uint32(16), // not used
			DwRtoMin:       uint32(17), // not used
			DwRtoMax:       uint32(18), // not used
			DwMaxConn:      uint32(19), // not used
			DwActiveOpens:  uint32(20),
			DwPassiveOpens: uint32(21),
			DwAttemptFails: uint32(22),
			DwEstabResets:  uint32(23),
			DwCurrEstab:    uint32(24),
			DwInSegs:       uint32(25),
			DwOutSegs:      uint32(26),
			DwRetransSegs:  uint32(27),
			DwInErrs:       uint32(28),
			DwOutRsts:      uint32(29),
			DwNumConns:     uint32(30),
		},
	}

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
excluded_interface_re: "eth[0-9]"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	eth1Tags := []string{"device:eth1"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", eth1Tags)

	lo0Tags := []string{"device:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(26), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(27), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(28), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(29), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(30), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(31), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(32), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(33), "", lo0Tags)
}
