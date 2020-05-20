// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-2020 Datadog, Inc.

// +build linux

package net

import (
	"testing"

	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/shirou/gopsutil/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type fakeNetworkStats struct {
	counterStats                []net.IOCountersStat
	counterStatsError           error
	protoCountersStats          []net.ProtoCountersStat
	protoCountersStatsError     error
	connectionStatsUDP4         []net.ConnectionStat
	connectionStatsUDP4Error    error
	connectionStatsUDP6         []net.ConnectionStat
	connectionStatsUDP6Error    error
	connectionStatsTCP4         []net.ConnectionStat
	connectionStatsTCP4Error    error
	connectionStatsTCP6         []net.ConnectionStat
	connectionStatsTCP6Error    error
	netstatTCPExtCountersValues map[string]int64
	netstatTCPExtCountersError  error
}

// IOCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return n.counterStats, n.counterStatsError
}

// ProtoCounters returns the inner values of counterStats and counterStatsError
func (n *fakeNetworkStats) ProtoCounters(protocols []string) ([]net.ProtoCountersStat, error) {
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

func (n *fakeNetworkStats) NetstatTCPExtCounters() (map[string]int64, error) {
	return n.netstatTCPExtCountersValues, n.netstatTCPExtCountersError
}

func TestDefaultConfiguration(t *testing.T) {
	check := NetworkCheck{}
	check.Configure([]byte(``), []byte(``), "test")

	assert.Equal(t, false, check.config.instance.CollectConnectionState)
	assert.Equal(t, []string(nil), check.config.instance.ExcludedInterfaces)
	assert.Equal(t, "", check.config.instance.ExcludedInterfaceRe)
}

func TestConfiguration(t *testing.T) {
	check := NetworkCheck{}
	rawInstanceConfig := []byte(`
collect_connection_state: true
excluded_interfaces:
    - eth0
    - lo0
excluded_interface_re: "eth.*"
`)
	err := check.Configure(rawInstanceConfig, []byte(``), "test")

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
				Errin:       13,
				PacketsSent: 14,
				Errout:      15,
			},
			{
				Name:        "lo0",
				BytesRecv:   16,
				BytesSent:   17,
				PacketsRecv: 18,
				Errin:       19,
				PacketsSent: 20,
				Errout:      21,
			},
		},
		protoCountersStats: []net.ProtoCountersStat{
			{
				Protocol: "tcp",
				Stats: map[string]int64{
					"RetransSegs": 22,
					"InSegs":      23,
					"OutSegs":     24,
				},
			},
			{
				Protocol: "udp",
				Stats: map[string]int64{
					"InDatagrams":  25,
					"NoPorts":      26,
					"InErrors":     27,
					"OutDatagrams": 28,
					"RcvbufErrors": 29,
					"SndbufErrors": 30,
					"InCsumErrors": 31,
				},
			},
		},
		connectionStatsUDP4: []net.ConnectionStat{
			{
				Status: "NONE",
			},
		},
		connectionStatsUDP6: []net.ConnectionStat{
			{
				Status: "NONE",
			},
			{
				Status: "NONE",
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
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
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
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
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
				Status: "SYN_RECV",
			},
			{
				Status: "FIN_WAIT1",
			},
			{
				Status: "FIN_WAIT2",
			},
			{
				Status: "TIME_WAIT",
			},
			{
				Status: "CLOSE",
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
		netstatTCPExtCountersValues: map[string]int64{
			"ListenOverflows": 32,
			"ListenDrops":     33,
			"TCPBacklogDrop":  34,
			"TCPRetransFail":  35,
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
collect_connection_state: true
`)

	err := networkCheck.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender := mocksender.NewMockSender(networkCheck.ID())

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	var customTags []string

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(15), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(16), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(17), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(18), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(19), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(20), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(21), "", lo0Tags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_segs", float64(22), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(23), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(24), "", customTags)

	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_segs.count", float64(22), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_segs.count", float64(23), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_segs.count", float64(24), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_datagrams", float64(25), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.no_ports", float64(26), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_errors", float64(27), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.out_datagrams", float64(28), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.rcv_buf_errors", float64(29), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.snd_buf_errors", float64(30), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.udp.in_csum_errors", float64(31), "", customTags)

	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_datagrams.count", float64(25), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.no_ports.count", float64(26), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_errors.count", float64(27), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.out_datagrams.count", float64(28), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.rcv_buf_errors.count", float64(29), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.snd_buf_errors.count", float64(30), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.udp.in_csum_errors.count", float64(31), "", customTags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)

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
				Errin:       13,
				PacketsSent: 14,
				Errout:      15,
			},
			{
				Name:        "lo0",
				BytesRecv:   16,
				BytesSent:   17,
				PacketsRecv: 18,
				Errin:       19,
				PacketsSent: 20,
				Errout:      21,
			},
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
excluded_interfaces:
    - lo0
`)

	networkCheck.Configure(rawInstanceConfig, []byte(``), "test")

	mockSender := mocksender.NewMockSender(networkCheck.ID())

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(15), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(16), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(17), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(18), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(19), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(20), "", lo0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(21), "", lo0Tags)
}

func TestExcludedInterfacesRe(t *testing.T) {
	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   10,
				BytesSent:   11,
				PacketsRecv: 12,
				Errin:       13,
				PacketsSent: 14,
				Errout:      15,
			},
			{
				Name:        "eth1",
				BytesRecv:   16,
				BytesSent:   17,
				PacketsRecv: 18,
				Errin:       19,
				PacketsSent: 20,
				Errout:      21,
			},
			{
				Name:        "lo0",
				BytesRecv:   22,
				BytesSent:   23,
				PacketsRecv: 24,
				Errin:       25,
				PacketsSent: 26,
				Errout:      27,
			},
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
excluded_interface_re: "eth[0-9]"
`)

	err := networkCheck.Configure(rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender := mocksender.NewMockSender(networkCheck.ID())

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(13), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(14), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(15), "", eth0Tags)

	eth1Tags := []string{"device:eth1", "device_name:eth1"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(16), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(17), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(18), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(19), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(20), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(21), "", eth1Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(22), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(23), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(24), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(25), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(26), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(27), "", lo0Tags)
}
