// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package networkv2 provides a check for network connection and socket statistics
package networkv2

import (
	"bufio"
	"bytes"
	"slices"
	"strings"
	"testing"

	gocmp "github.com/google/go-cmp/cmp"
	"github.com/shirou/gopsutil/v4/net"
	"github.com/spf13/afero"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/safchain/ethtool"
)

type fakeNetworkStats struct {
	counterStats                 []net.IOCountersStat
	counterStatsError            error
	protoCountersStats           []net.ProtoCountersStat
	protoCountersStatsError      error
	connectionStatsUDP4          []net.ConnectionStat
	connectionStatsUDP4Error     error
	connectionStatsUDP6          []net.ConnectionStat
	connectionStatsUDP6Error     error
	connectionStatsTCP4          []net.ConnectionStat
	connectionStatsTCP4Error     error
	connectionStatsTCP6          []net.ConnectionStat
	connectionStatsTCP6Error     error
	netstatAndSnmpCountersValues map[string]net.ProtoCountersStat
	netstatAndSnmpCountersError  error
	getProcPath                  string
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

func (n *fakeNetworkStats) NetstatAndSnmpCounters(_ []string) (map[string]net.ProtoCountersStat, error) {
	return n.netstatAndSnmpCountersValues, n.netstatAndSnmpCountersError
}

func (n *fakeNetworkStats) GetProcPath() string {
	return n.getProcPath
}

func (n *fakeNetworkStats) GetNetProcBasePath() string {
	return n.getProcPath
}

type MockEthtool struct {
	mock.Mock
}

func (f *MockEthtool) DriverInfo(intf string) (ethtool.DrvInfo, error) {
	if intf == "eth0" {
		return ethtool.DrvInfo{
			Driver:  "ena",
			Version: "mock_version",
		}, nil
	}

	return ethtool.DrvInfo{}, unix.ENOTTY
}

func (f *MockEthtool) Stats(intf string) (map[string]uint64, error) {
	if intf == "eth0" {
		return map[string]uint64{
			"queue_0_tx_packets": 12345,
			"rx_packets[0]":      67890,
			"cpu0_rx_xdp_tx":     123,
			"tx_timeout":         456,
		}, nil
	}

	return nil, unix.ENOTTY
}

type MockCommandRunner struct {
	mock.Mock
}

func (m *MockCommandRunner) FakeRunCommand(cmd []string, _ []string) (string, error) {
	if slices.Contains(cmd, "netstat") {
		return `Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp         0      0 46.105.75.4:80          79.220.227.193:2032     TIME_WAIT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp         0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     TIME_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_SENT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     SYN_RECV
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT1
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     FIN_WAIT2
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     TIME_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSE_WAIT
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LAST_ACK
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     LISTEN
tcp6        0      0 46.105.75.4:143         90.56.111.177:56867     CLOSING
udp         0      0 46.105.75.4:143         90.56.111.177:56867
udp6        0      0 46.105.75.4:143         90.56.111.177:56867     ESTABLISHED
udp6        0      0 46.105.75.4:143         90.56.111.177:56867
`, nil
	} else if slices.ContainsFunc(cmd, func(s string) bool {
		return strings.Contains(s, "ss")
	}) {
		return `State     Recv-Q    Send-Q    Local Address           Foreign Address
ESTAB     0         0         127.0.0.1:60342         127.0.0.1:46153
TIME-WAIT 0         0         127.0.0.1:46153         127.0.0.1:60342
`, nil
	}
	return `cpu=0 found=27644 invalid=19060 ignore=485633411 insert=0 count=42 drop=1 early_drop=0 max=42 search_restart=39936711
	cpu=1 found=21960 invalid=17288 ignore=475938848 insert=0 count=42 drop=1 early_drop=0 max=42 search_restart=36983181`, nil
}

func createTestNetworkCheck(mockNetStats networkStats) *NetworkCheck {
	return &NetworkCheck{
		net: mockNetStats,
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics:        true,
				WhitelistConntrackMetrics: []string{"max", "count"},
				UseSudoConntrack:          true,
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
collect_count_metrics: true
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
		netstatAndSnmpCountersValues: map[string]net.ProtoCountersStat{
			"Tcp": {
				Protocol: "Tcp",
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
			"Udp": {
				Protocol: "Udp",
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
			"TcpExt": {
				Protocol: "TcpExt",
				Stats: map[string]int64{
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
	}

	mockEthtool := new(MockEthtool)
	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getEthtoolDrvInfo = mockEthtool.DriverInfo
	getEthtoolStats = mockEthtool.Stats

	ssAvailableFunction = func() bool { return false }

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand
	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
collect_connection_state: true
collect_count_metrics: true
collect_ethtool_stats: true
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")
	assert.Nil(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	var customTags []string

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
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

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
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

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err = afero.WriteFile(fs, "/sys/class/net/eth0/speed", []byte(
		`10000`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/sys/class/net/eth0/mtu", []byte(
		`1500`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Nil(t, err)

	eth0Tags := []string{"device:eth0", "device_name:eth0", "speed:10000", "mtu:1500"}
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
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getEthtoolDrvInfo = mockEthtool.DriverInfo
	getEthtoolStats = mockEthtool.Stats

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

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_stats: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", float64(12345), "", expectedTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.queue.rx_packets", float64(67890), "", expectedTags)
	expectedTagsCPU := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "cpu:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.cpu.rx_xdp_tx", float64(123), "", expectedTagsCPU)
	expectedTagsGlobal := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "global"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.ena.tx_timeout", float64(456), "", expectedTagsGlobal)
}

func TestFetchEthtoolStatsENOTTY(t *testing.T) {
	mockEthtool := new(MockEthtool)

	mockEthtool.On("getDriverInfo", mock.Anything).Return(ethtool.DrvInfo{}, nil)
	mockEthtool.On("Stats", mock.Anything).Return(map[string]int{}, nil)

	getEthtoolDrvInfo = mockEthtool.DriverInfo
	getEthtoolStats = mockEthtool.Stats

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

	networkCheck := createTestNetworkCheck(net)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(`collect_ethtool_stats: true`), []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	expectedTagsIfNoError := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "queue:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.tx_packets", float64(12345), "", expectedTagsIfNoError)
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.queue.rx_packets", float64(67890), "", expectedTagsIfNoError)
	expectedTagsCPUIfNoError := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "cpu:0"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.cpu.rx_xdp_tx", float64(123), "", expectedTagsCPUIfNoError)
	expectedTagsGlobal := []string{"interface:eth0", "driver_name:ena", "driver_version:mock_version", "global"}
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.ena.tx_timeout", float64(456), "", expectedTagsGlobal)
}

func TestNetstatAndSnmpCountersUsingCorrectMockedProcfsPath(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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
		`TcpExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TcpExt: 32 33 34 35
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

func TestNetstatAndSnmpCountersWrongConfiguredLocation(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/wrong_mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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
		`TcpExt: ListenOverflows ListenDrops TCPBacklogDrop TCPRetransFail
TcpExt: 32 33 34 35
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)

	err = networkCheck.Run()
	assert.Equal(t, err, nil)
}

func TestNetstatAndSnmpCountersNoColonFile(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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

func TestNetstatAndSnmpCountersBadDataLine(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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
		`TcpExt: `),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, not data line")
}

func TestNetstatAndSnmpCountersMismatchedColumns(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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
		`TcpExt: 1 0 46 79
TcpExt: 32 34 192
IpExt: 800 4343 4342 304
IpExt: 801 439 120 439`),
		0644)
	assert.Nil(t, err)
	_ = networkCheck.Run()

	w.Flush()
	assert.Contains(t, b.String(), "/mocked/procfs/net/netstat is not fomatted correctly, expected same number of columns")
}

func TestNetstatAndSnmpCountersLettersForNumbers(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

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
		`TcpExt: 1 0 46 79
TcpExt: ab cd ef gh
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

func TestConntrackMonotonicCount(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_insert", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	expectedTags := []string{"cpu:0"}
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.conntrack.count", float64(42), "", expectedTags)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.conntrack.max", float64(42), "", expectedTags)
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.conntrack.ignore_this", mock.Anything, mock.Anything, mock.Anything)
}

func TestConntrackGaugeBlacklist(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
whitelist_conntrack_metrics: ["max", "count"]
blacklist_conntrack_metrics: ["count", "entries", "max"]
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_max", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_count", []byte(
		`14`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.max", float64(13), "", []string{})
	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.count", float64(13), "", []string{})
}

func TestConntrackGaugeWhitelist(t *testing.T) {
	net := &defaultNetworkStats{procPath: "/mocked/procfs"}
	networkCheck := createTestNetworkCheck(net)

	rawInstanceConfig := []byte(`
procfs_path: "/mocked/procfs"
collect_conntrack_metrics: true
conntrack_path: "/usr/bin/conntrack"
whitelist_conntrack_metrics: ["max", "include"]
`)

	mockSender := mocksender.NewMockSender(networkCheck.ID())
	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, rawInstanceConfig, []byte(``), "test")

	filesystem = afero.NewMemMapFs()
	fs := filesystem
	err := afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_insert", []byte(
		`13`),
		0644)
	assert.Nil(t, err)
	err = afero.WriteFile(fs, "/mocked/procfs/sys/net/netfilter/nf_conntrack_include", []byte(
		`14`),
		0644)
	assert.Nil(t, err)
	err = networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertNotCalled(t, "Gauge", "system.net.conntrack.insert", float64(13), "", []string{})
	mockSender.AssertMetric(t, "Gauge", "system.net.conntrack.include", float64(14), "", []string{})
}

func TestFetchQueueStatsSS(t *testing.T) {
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

	ssAvailableFunction = func() bool { return true }
	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)

	fakeInstanceConfig := []byte(`conntrack_path: ""
collect_connection_state: true
collect_connection_queues: true`)
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, fakeInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:established"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:established"})
}

func TestFetchQueueStatsNetstat(t *testing.T) {
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

	ssAvailableFunction = func() bool { return false }
	mockCommandRunner := new(MockCommandRunner)
	runCommandFunction = mockCommandRunner.FakeRunCommand

	mockCommandRunner.On("FakeRunCommand", mock.Anything, mock.Anything).Return([]byte("0"), nil)

	networkCheck := createTestNetworkCheck(net)
	fakeInstanceConfig := []byte(`conntrack_path: ""
collect_connection_state: true
collect_connection_queues: true`)
	mockSender := mocksender.NewMockSender(networkCheck.ID())
	networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, fakeInstanceConfig, []byte(``), "test")

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Histogram", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	err := networkCheck.Run()
	assert.Nil(t, err)

	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.send_q", float64(0), "", []string{"state:established"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:time_wait"})
	mockSender.AssertCalled(t, "Histogram", "system.net.tcp.recv_q", float64(0), "", []string{"state:established"})
}

func TestParseSocketStatMetrics(t *testing.T) {
	testcases := []struct {
		name     string
		protocol string
		input    string
		want     map[string]*connectionStateEntry
	}{
		{
			name:     "initializes tcp4 states",
			protocol: "tcp4",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes tcp6 states",
			protocol: "tcp6",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp4 states",
			protocol: "udp4",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp6 states",
			protocol: "udp6",
			input: `
State                  Recv-Q              Send-Q                                 Local Address:Port                              Peer Address:Port
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp4 states correctly",
			protocol: "tcp4",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
LISTEN         0           4096           127.0.0.53%lo:53             0.0.0.0:*
LISTEN         1024        0                   0.0.0.0:27500          0.0.0.0:*
LISTEN         0           4096              127.0.0.54:53             0.0.0.0:*
ESTAB          0           0               192.168.64.6:38848    34.107.243.93:443
TIME-WAIT      0           0        192.168.64.6%enp0s1:42804     38.145.32.21:80
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"listening": {
					count: 3,
					recvQ: []uint64{0, 1024, 0},
					sendQ: []uint64{4096, 0, 4096},
				},
			},
		},
		{
			name:     "collects tcp6 states correctly",
			protocol: "tcp6",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
LISTEN         0           4096           127.0.0.53%lo:53             0.0.0.0:*
LISTEN         1024           0                   0.0.0.0:27500          0.0.0.0:*
ESTAB          0           0               192.168.64.6:38848    34.107.243.93:443
TIME-WAIT      0           0        192.168.64.6%enp0s1:42804     38.145.32.21:80
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
				"listening": {
					count: 2,
					recvQ: []uint64{0, 1024},
					sendQ: []uint64{4096, 0},
				},
			},
		},
		{
			name:     "collects udp4 states correctly",
			protocol: "udp4",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
UNCONN      0           0           127.0.0.53%lo:53             0.0.0.0:*
UNCONN      0           0                   0.0.0.0:27500          0.0.0.0:*
UNCONN      0           0              127.0.0.54:53             0.0.0.0:*
UNCONN      0           0                 0.0.0.0:5355           0.0.0.0:*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 4,
					recvQ: []uint64{0, 0, 0, 0},
					sendQ: []uint64{0, 0, 0, 0},
				},
			},
		},
		{
			name:     "collects udp6 states correctly",
			protocol: "udp6",
			input: `
State          Recv-Q      Send-Q         Local Address:Port      Peer Address:Port
UNCONN      0           0           127.0.0.53%lo:53             0.0.0.0:*
UNCONN      0           0                   0.0.0.0:27500          0.0.0.0:*
UNCONN      0           0              127.0.0.54:53             0.0.0.0:*
UNCONN      0           0                 0.0.0.0:5355           0.0.0.0:*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 4,
					recvQ: []uint64{0, 0, 0, 0},
					sendQ: []uint64{0, 0, 0, 0},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseSocketStatsMetrics(tc.protocol, tc.input)
			assert.NoError(t, err)
			if diff := gocmp.Diff(tc.want, got, gocmp.Comparer(connectionStateEntryComparer)); diff != "" {
				t.Errorf("socket statistics result parsing diff (-want +got):\n%s", diff)
			}
		})
	}
}

func TestParseNetstatMetrics(t *testing.T) {
	testcases := []struct {
		name     string
		protocol string
		input    string
		want     map[string]*connectionStateEntry
	}{
		{
			name:     "initializes tcp4 states",
			protocol: "tcp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes tcp6 states",
			protocol: "tcp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening":   emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp4 states",
			protocol: "udp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "initializes udp6 states",
			protocol: "udp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
`,
			want: map[string]*connectionStateEntry{
				"connections": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp4 states correctly",
			protocol: "tcp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        1024      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      1024 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      1024 :::5355                 :::*                    LISTEN
tcp6       1024      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"established": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{1024},
				},
				"opening": emptyConnectionStateEntry(),
				"closing": emptyConnectionStateEntry(),
				"time_wait": {
					count: 1,
					recvQ: []uint64{1024},
					sendQ: []uint64{0},
				},
				"listening": emptyConnectionStateEntry(),
			},
		},
		{
			name:     "collects tcp6 states correctly",
			protocol: "tcp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        1024      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      1024 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      1024 :::5355                 :::*                    LISTEN
tcp6       1024      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"established": emptyConnectionStateEntry(),
				"opening":     emptyConnectionStateEntry(),
				"closing":     emptyConnectionStateEntry(),
				"time_wait":   emptyConnectionStateEntry(),
				"listening": {
					count: 2,
					recvQ: []uint64{0, 1024},
					sendQ: []uint64{1024, 0},
				},
			},
		},
		{
			name:     "collects udp4 states correctly",
			protocol: "udp4",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      0 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      0 :::5355                 :::*                    LISTEN
tcp6       0      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 2,
					recvQ: []uint64{0, 0},
					sendQ: []uint64{0, 0},
				},
			},
		},
		{
			name:     "collects udp6 states correctly",
			protocol: "udp6",
			input: `
Active Internet connections (servers and established)
Proto Recv-Q Send-Q Local Address           Foreign Address         State
tcp        0      0 192.168.64.6:34816      34.49.51.44:443         TIME_WAIT
tcp        0      0 192.168.64.6:33852      34.107.243.93:443       ESTABLISHED
tcp6       0      0 :::5355                 :::*                    LISTEN
tcp6       0      0 ::1:631                 :::*                    LISTEN
udp        0      0 127.0.0.53:53           0.0.0.0:*
udp        0      0 192.168.64.6:68         192.168.64.1:67         ESTABLISHED
udp6       0      0 :::5353                 :::*
`,
			want: map[string]*connectionStateEntry{
				"connections": {
					count: 1,
					recvQ: []uint64{0},
					sendQ: []uint64{0},
				},
			},
		},
	}

	for _, tc := range testcases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseNetstatMetrics(tc.protocol, tc.input)
			assert.NoError(t, err)
			if diff := gocmp.Diff(tc.want, got, gocmp.Comparer(connectionStateEntryComparer)); diff != "" {
				t.Errorf("netstat result parsing diff (-want +got):\n%s", diff)
			}
		})
	}
}

func connectionStateEntryComparer(a, b *connectionStateEntry) bool {
	return a.count == b.count &&
		gocmp.Equal(a.recvQ, b.recvQ) &&
		gocmp.Equal(a.sendQ, b.sendQ)
}

func emptyConnectionStateEntry() *connectionStateEntry {
	return &connectionStateEntry{
		count: 0,
		recvQ: []uint64{},
		sendQ: []uint64{},
	}
}
