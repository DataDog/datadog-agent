// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"bufio"
	"bytes"
	"errors"
	"syscall"
	"testing"
	"unsafe"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
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
//
//nolint:revive // TODO(PLINT) Fix revive linter
func (n *fakeNetworkStats) IOCounters(pernic bool) ([]net.IOCountersStat, error) {
	return n.counterStats, n.counterStatsError
}

// ProtoCounters returns the inner values of counterStats and counterStatsError
//
//nolint:revive // TODO(PLINT) Fix revive linter
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

type MockSyscall struct {
	mock.Mock
}

func (m *MockSyscall) Socket(domain, typ, protocol int) (fd int, err error) {
	args := m.Called(domain, typ, protocol)
	return args.Int(0), args.Error(1)
}

//nolint:govet
func (m *MockSyscall) Syscall(trap, a1, a2, a3 uintptr) (r1, r2 uintptr, err syscall.Errno) {
	args := m.Called(trap, a1, a2, a3)
	if trap == unix.SYS_IOCTL {
		cmd := a2
		req := (*ifreq)(unsafe.Pointer(a3))

		switch cmd {
		case uintptr(ETHTOOL_GDRVINFO):
			if string(bytes.Trim(req.Name[:], "\x00")) == "virtual_iface" {
				return 0, 0, 25
			}
			drvInfo := (*ethtool_drvinfo)(unsafe.Pointer(req.Data))
			drvInfo.Driver = "mock_driver"
			drvInfo.Version = "mock_version"
			return 0, 0, 0

		case uintptr(ETHTOOL_GSSET_INFO):
			stringSet := (*ethtool_gstrings)(unsafe.Pointer(req.Data))
			stringSet.Len = 2
			return 0, 0, 0

		case uintptr(ETHTOOL_GSTRINGS):
			stringSet := (*ethtool_gstrings)(unsafe.Pointer(req.Data))
			mockStats := []string{"tx_packets", "rx_bytes"}
			for i, stat := range mockStats {
				copy(stringSet.Data[i*ETH_GSTRING_LEN:(i+1)*ETH_GSTRING_LEN], stat)
			}
			return 0, 0, 0

		case uintptr(ETHTOOL_GSTATS):
			stats := (*ethtool_stats)(unsafe.Pointer(req.Data))
			stats.Data[0] = 12345
			stats.Data[1] = 67890
			return 0, 0, 0
		}
	}
	return args.Get(0).(uintptr), args.Get(1).(uintptr), syscall.Errno(0)
}

func (m *MockSyscall) Close(fd int) error {
	args := m.Called(fd)
	return args.Error(0)
}

func TestDefaultConfiguration(t *testing.T) {
	check := NetworkCheck{}
	check.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

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
		protoCountersStats: []net.ProtoCountersStat{
			{
				Protocol: "tcp",
				Stats: map[string]int64{
					"RetransSegs":  22,
					"InSegs":       23,
					"OutSegs":      24,
					"ActiveOpens":  39,
					"PassiveOpens": 40,
					"AttemptFails": 41,
					"EstabResets":  42,
					"InErrs":       36,
					"OutRsts":      37,
					"InCsumErrors": 38,
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
			"ListenOverflows":      32,
			"ListenDrops":          33,
			"TCPBacklogDrop":       34,
			"TCPRetransFail":       35,
			"IPReversePathFilter":  43,
			"PruneCalled":          44,
			"RcvPruned":            45,
			"OfoPruned":            46,
			"PAWSActive":           47,
			"PAWSEstab":            48,
			"SyncookiesSent":       49,
			"SyncookiesRecv":       50,
			"SyncookiesFailed":     51,
			"TCPAbortOnTimeout":    52,
			"TCPSynRetrans":        53,
			"TCPFromZeroWindowAdv": 54,
			"TCPToZeroWindowAdv":   55,
			"TWRecycled":           56,
		},
	}

	mockSyscall := new(MockSyscall)

	mockSyscall.On("Socket", mock.Anything, mock.Anything, mock.Anything).Return(1, nil)
	mockSyscall.On("Syscall", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(uintptr(0), uintptr(0), syscall.Errno(0))
	mockSyscall.On("Close", mock.Anything).Return(nil)

	getSyscall = mockSyscall.Syscall
	getSocket = mockSyscall.Socket
	getClose = mockSyscall.Close

	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
collect_connection_state: true
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

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", lo0Tags)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_segs", float64(22), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_segs", float64(23), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_segs", float64(24), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.active_opens", float64(39), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.passive_opens", float64(40), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.attempt_fails", float64(41), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.established_resets", float64(42), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_errors", float64(36), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.out_resets", float64(37), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.in_csum_errors", float64(38), "", customTags)

	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_segs.count", float64(22), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_segs.count", float64(23), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_segs.count", float64(24), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.active_opens.count", float64(39), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.passive_opens.count", float64(40), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.attempt_fails.count", float64(41), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.established_resets.count", float64(42), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_errors.count", float64(36), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.out_resets.count", float64(37), "", customTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.in_csum_errors.count", float64(38), "", customTags)

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
	mockSender.AssertCalled(t, "Rate", "system.net.ip.reverse_path_filter", float64(43), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_called", float64(44), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_rcv_drops", float64(45), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.prune_ofo_called", float64(46), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.paws_connection_drops", float64(47), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.paws_established_drops", float64(48), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_sent", float64(49), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_recv", float64(50), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_cookies_failed", float64(51), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.abort_on_timeout", float64(52), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.syn_retrans", float64(53), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.from_zero_window", float64(54), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.to_zero_window", float64(55), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.tw_reused", float64(56), "", customTags)

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
	}

	networkCheck := NetworkCheck{
		net: net,
	}

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

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
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
	}

	networkCheck := NetworkCheck{
		net: net,
	}

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

	eth0Tags := []string{"device:eth0", "device_name:eth0"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(10), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(11), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(12), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(13), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(14), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(15), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(16), "", eth0Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(17), "", eth0Tags)

	eth1Tags := []string{"device:eth1", "device_name:eth1"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", float64(18), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_sent", float64(19), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.count", float64(20), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.drop", float64(21), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_in.error", float64(22), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.count", float64(23), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.drop", float64(24), "", eth1Tags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.packets_out.error", float64(25), "", eth1Tags)

	lo0Tags := []string{"device:lo0", "device_name:lo0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(26), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(27), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(28), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(29), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(30), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(31), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(32), "", lo0Tags)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(33), "", lo0Tags)
}

func TestFetchEthtoolStats(t *testing.T) {
	mockSyscall := new(MockSyscall)

	mockSyscall.On("Socket", mock.Anything, mock.Anything, mock.Anything).Return(1, nil)
	mockSyscall.On("Syscall", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(uintptr(0), uintptr(0), syscall.Errno(0))
	mockSyscall.On("Close", mock.Anything).Return(nil)

	getSyscall = mockSyscall.Syscall
	getSocket = mockSyscall.Socket
	getClose = mockSyscall.Close

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "eth0",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"interface:eth0", "driver_name:mock_driver", "driver_version:mock_version"}
	mockSender.AssertCalled(t, "Rate", "system.net.tx_packets", float64(12345), "", expectedTags)
	mockSender.AssertCalled(t, "Rate", "system.net.rx_bytes", float64(67890), "", expectedTags)
}

func TestFetchEthtoolStatsErrorBadSocket(t *testing.T) {
	mockSyscall := new(MockSyscall)

	getSyscall = mockSyscall.Syscall
	getSocket = func(domain int, typ int, proto int) (int, error) {
		return 0, errors.New("bad socket")
	}
	getClose = mockSyscall.Close

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "virtual_iface",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Equal(t, errors.New("bad socket"), err)
}

func TestFetchEthtoolStatsENOTTY(t *testing.T) {
	mockSyscall := new(MockSyscall)

	mockSyscall.On("Socket", mock.Anything, mock.Anything, mock.Anything).Return(1, nil)
	mockSyscall.On("Syscall", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(uintptr(0), uintptr(0), syscall.Errno(0))
	mockSyscall.On("Close", mock.Anything).Return(nil)

	getSyscall = mockSyscall.Syscall
	getSocket = mockSyscall.Socket
	getClose = mockSyscall.Close

	net := &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{
				Name:        "virtual_iface",
				BytesRecv:   100,
				BytesSent:   200,
				PacketsRecv: 300,
				Dropin:      400,
				Errin:       500,
				PacketsSent: 600,
				Dropout:     700,
				Errout:      800,
			},
		},
	}

	networkCheck := NetworkCheck{
		net: net,
	}

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTagsIfNoError := []string{"interface:virtual_iface", "driver_name:mock_driver", "driver_version:mock_version"}
	mockSender.AssertNotCalled(t, "Rate", "system.net.tx_packets", float64(12345), "", expectedTagsIfNoError)
	mockSender.AssertNotCalled(t, "Rate", "system.net.rx_bytes", float64(67890), "", expectedTagsIfNoError)
}

func TestNetStatTCPExtCountersUsingCorrectMockedProcfsPath(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)
	var customTags []string

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TCPExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TCPExt: 32 33 34 35
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)
}

func TestNetStatTCPExtCountersWrongConfiguredLocation(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/wrong_mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/wrong_mocked/procfs"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TCPExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TCPExt: 32 33 34 35
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Equal(t, err, nil)
}

func TestNetStatTCPExtCountersNoColonFile(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`bad file`),
		0644)
	assert.Nil(t, err)

	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, expected ':'")
}

func TestNetStatTCPExtCountersBadDataLine(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TCPExt: `),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, not data line")
}

func TestNetStatTCPExtCountersMismatchedColumns(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)

	var b bytes.Buffer
	w := bufio.NewWriter(&b)

	logger, err := log.LoggerFromWriterWithMinLevelAndFormat(w, log.DebugLvl, "[%LEVEL] %Msg")
	assert.Nil(t, err)
	log.SetupLogger(logger, "debug")
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err = networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TCPExt: 1 0 46 79
TCPExt: 32 34 192
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, expected same number of columns")
}

func TestNetStatTCPExtCountersLettersForNumbers(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := NetworkCheck{
		net: net,
	}

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
`)
	var customTags []string

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/mocked/procfs/net/netstat", []byte(
		`TCPExt: 1 0 46 79
TCPExt: ab cd ef gh
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),

		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.listen_overflows", float64(32), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.listen_drops", float64(33), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.backlog_drops", float64(34), "", customTags)
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.failed_retransmits", float64(35), "", customTags)
}
