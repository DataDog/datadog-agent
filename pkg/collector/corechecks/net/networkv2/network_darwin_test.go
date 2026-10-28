// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package networkv2

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	inventorychecksmock "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/networkv2/nicinfo"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

const netstatTCPStats = `tcp:
	3651535 packets sent
		972097 data packets (615753248 bytes)
		5009 data packets (2832232 bytes) retransmitted
		0 resends initiated by MTU discovery
	4807551 packets received
		1143534 acks (for 616095538 bytes)
`

// netstatTCPStatsUnprivileged is what macOS reports to users other than root
const netstatTCPStatsUnprivileged = `tcp:
	0 packet sent
		0 data packet (0 byte)
		0 data packet (0 byte) retransmitted
	0 packet received
`

const netstatTCPSockets = `Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
tcp4       0      0  192.168.1.130.57002    3.233.158.68.443       ESTABLISHED
tcp4       0      0  192.168.1.130.57003    3.233.158.68.443       SYN_RCVD
tcp6       0      0  *.22                   *.*                    LISTEN
tcp46      0      0  *.8080                 *.*                    LISTEN
tcp4       0      0  127.0.0.1.5001         127.0.0.1.62000        TIME_WAIT
tcp4       0      0  127.0.0.1.5002         127.0.0.1.62001        FIN_WAIT_2
tcp4       0      0  127.0.0.1.5003         127.0.0.1.62002        CLOSED
`

const netstatUDPSockets = `Active Internet connections (including servers)
Proto Recv-Q Send-Q  Local Address          Foreign Address        (state)
udp4       0      0  *.5353                 *.*
udp46      0      0  *.123                  *.*
udp6       0      0  *.5353                 *.*
`

type fakeNetworkStats struct {
	interfaces    []darwinInterface
	interfacesErr error
	netstat       map[string]string
	netstatErr    error
}

// Interfaces returns the mocked interfaces
func (n *fakeNetworkStats) Interfaces() ([]darwinInterface, error) {
	return n.interfaces, n.interfacesErr
}

// Netstat returns the mocked output of netstat for args
func (n *fakeNetworkStats) Netstat(args ...string) (string, error) {
	return n.netstat[strings.Join(args, " ")], n.netstatErr
}

// newFakeNetworkStats returns interfaces as a Mac with Wi-Fi, a USB Ethernet adapter, an unused
// built-in Ethernet port, a USB device-mode interface, a VPN tunnel, a VLAN and a Thunderbolt
// bridge would report them.
func newFakeNetworkStats() *fakeNetworkStats {
	return &fakeNetworkStats{
		interfaces: []darwinInterface{
			{
				name: "lo0", flags: unix.IFF_UP | unix.IFF_LOOPBACK | unix.IFF_RUNNING, ifType: unix.IFT_LOOP,
				functionalType: functionalTypeLoopback, mtu: 16384,
				ipackets: 10, ibytes: 1000, opackets: 10, obytes: 1000,
			},
			{
				name: "en0", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_ETHER,
				functionalType: functionalTypeWifiInfra, macAddress: "9a:62:9c:bd:37:0a", mtu: 1500, baudrate: 866_700_000,
				media:    &ifMedia{status: ifmAvalid | ifmActive, active: 0x80},
				ipackets: 50, ierrors: 1, iqdrops: 2, ibytes: 5000, opackets: 40, oerrors: 3, sendDrops: 6, obytes: 4000,
				nic: nicinfo.Info{
					Description: "Wi-Fi", PermanentMAC: "bc:d0:74:1e:bd:b7", Builtin: pointer.Ptr(true),
					DriverClass: "AppleBCMWLANSkywalkInterface", Driver: "com.apple.DriverKit-AppleBCMWLAN",
				},
			},
			{
				name: "en7", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_ETHER,
				functionalType: functionalTypeWired, macAddress: "00:e0:4c:68:01:02", mtu: 1500, baudrate: 1_000_000_000,
				media:    &ifMedia{status: ifmAvalid | ifmActive, active: ifmEther | ifmFdx | 16},
				ipackets: 2, ibytes: 10, opackets: 2, obytes: 20,
				nic: nicinfo.Info{
					Description: "USB 10/100/1000 LAN", PermanentMAC: "00:e0:4c:68:01:02", Builtin: pointer.Ptr(false),
					Vendor: "Realtek", Model: "USB 10/100/1000 LAN", DriverClass: "AppleUserECM", Driver: "com.apple.DriverKit-AppleUserECM",
				},
			},
			{
				name: "en5", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_ETHER,
				functionalType: functionalTypeWired, macAddress: "a4:fc:14:0d:5e:21", mtu: 1500,
				media: &ifMedia{status: ifmAvalid, active: ifmEther | 2},
				nic: nicinfo.Info{
					Description: "Ethernet", PermanentMAC: "a4:fc:14:0d:5e:21", Builtin: pointer.Ptr(true),
					DriverClass: "AppleEthernetAquantiaAqtion", Driver: "com.apple.driver.AppleEthernetAquantiaAqtion", DriverVersion: "1.0.64",
				},
			},
			{
				name: "en4", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_ETHER,
				functionalType: functionalTypeWired, macAddress: "42:52:1c:e5:c2:ec", mtu: 1500,
				media: &ifMedia{status: ifmAvalid, active: ifmEther | 2},
				nic: nicinfo.Info{
					Description: "Ethernet Adapter (en4)", PermanentMAC: "42:52:1c:e5:c2:ec", Builtin: pointer.Ptr(false),
					DriverClass: "AppleUSBDeviceNCMData", Driver: "com.apple.driver.AppleUSBDeviceNCM", DriverVersion: "5.0.0",
				},
			},
			{
				name: "utun3", flags: unix.IFF_UP | unix.IFF_POINTOPOINT | unix.IFF_RUNNING, ifType: unix.IFT_OTHER, mtu: 1000,
				opackets: 1, obytes: 80,
			},
			{
				name: "vlan0", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_L2VLAN,
				functionalType: functionalTypeWired, macAddress: "00:e0:4c:68:01:02", mtu: 1500, baudrate: 1_000_000_000, vlanID: 10,
				media: &ifMedia{status: ifmAvalid | ifmActive, active: ifmEther | ifmFdx},
			},
			{
				name: "bridge0", flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING, ifType: unix.IFT_BRIDGE,
				functionalType: functionalTypeWired, macAddress: "36:80:7e:c5:c4:00", mtu: 1500,
				media: &ifMedia{status: ifmAvalid, active: 2},
				nic:   nicinfo.Info{Description: "Thunderbolt Bridge"},
			},
		},
		netstat: map[string]string{
			"-s -p tcp":    netstatTCPStats,
			"-n -a -p tcp": netstatTCPSockets,
			"-n -a -p udp": netstatUDPSockets,
		},
	}
}

// createTestNetworkCheck returns a check with the default configuration that reads netStats.
func createTestNetworkCheck(netStats networkStats) *NetworkCheck {
	return &NetworkCheck{
		CheckBase: core.NewCheckBase(CheckName),
		net:       netStats,
		config: networkConfig{
			instance: networkInstanceConfig{
				CollectRateMetrics:      true,
				CombineConnectionStates: true,
			},
		},
	}
}

// runCheck configures and runs a check on netStats with a sender that accepts every metric.
func runCheck(t *testing.T, netStats *fakeNetworkStats, rawInstanceConfig string) (*NetworkCheck, *mocksender.MockSender, error) {
	networkCheck := createTestNetworkCheck(netStats)
	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(rawInstanceConfig), []byte(``), "test", "provider")
	require.NoError(t, err)
	mockSender.SetupAcceptAll()
	return networkCheck, mockSender, networkCheck.Run()
}

// TestDefaultConfiguration tests the configuration of an instance without options.
func TestDefaultConfiguration(t *testing.T) {
	networkCheck := createTestNetworkCheck(nil)
	err := networkCheck.Configure(aggregator.NewNoOpSenderManager(), integration.FakeConfigHash, []byte(``), []byte(``), "test", "provider")
	require.NoError(t, err)

	assert.True(t, networkCheck.config.instance.CollectRateMetrics)
	assert.False(t, networkCheck.config.instance.CollectCountMetrics)
	assert.False(t, networkCheck.config.instance.CollectConnectionState)
	assert.True(t, networkCheck.config.instance.CombineConnectionStates)
	assert.Nil(t, networkCheck.config.instance.ExcludedInterfacePattern)
}

// TestInterfaceRates tests the traffic rates, which interfaces that never carried traffic do not get.
func TestInterfaceRates(t *testing.T) {
	_, mockSender, err := runCheck(t, newFakeNetworkStats(), `
excluded_interface_re: bridge.*
`)
	require.NoError(t, err)

	en0 := []string{"device:en0"}
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(5000), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(4000), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.count", float64(50), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.drop", float64(2), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_in.error", float64(1), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.count", float64(40), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.drop", float64(6), "", en0)
	mockSender.AssertCalled(t, "Rate", "system.net.packets_out.error", float64(3), "", en0)

	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(1000), "", []string{"device:lo0"})
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_sent", float64(80), "", []string{"device:utun3"})
	for _, idle := range []string{"device:en4", "device:en5", "device:vlan0", "device:bridge0"} {
		mockSender.AssertNotCalled(t, "Rate", "system.net.bytes_rcvd", mock.Anything, "", []string{idle})
	}
}

// TestDarwinIfaceMetrics tests the interface gauges of each kind of interface, and that excluded
// interfaces get none.
func TestDarwinIfaceMetrics(t *testing.T) {
	_, mockSender, err := runCheck(t, newFakeNetworkStats(), `
excluded_interfaces:
  - bridge0
`)
	require.NoError(t, err)

	wifiTags := []string{"iface:en0", "iface_type:wifi"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(1500), "", wifiTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", wifiTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", wifiTags)

	ethernetTags := []string{"iface:en7", "iface_type:ethernet", "duplex:full"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", ethernetTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.speed", float64(1000), "", ethernetTags)

	unpluggedTags := []string{"iface:en5", "iface_type:ethernet"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(1500), "", unpluggedTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(0), "", unpluggedTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", unpluggedTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(0), "", []string{"iface:en4", "iface_type:virtual"})

	tunnelTags := []string{"iface:utun3", "iface_type:tunnel"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", tunnelTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", tunnelTags)

	vlanTags := []string{"iface:vlan0", "iface_type:virtual", "duplex:full", "vlan_id:10"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", vlanTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", vlanTags)

	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", []string{"iface:lo0", "iface_type:loopback"})
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.up", mock.Anything, "", []string{"iface:bridge0", "iface_type:virtual"})
}

// TestDarwinInterfacesMetadata tests the "interfaces" check metadata, with the IOKit details.
func TestDarwinInterfacesMetadata(t *testing.T) {
	invChecks := inventorychecksmock.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	defer check.ReleaseContext()

	networkCheck, _, err := runCheck(t, newFakeNetworkStats(), ``)
	require.NoError(t, err)

	metadata := invChecks.GetInstanceMetadata(string(networkCheck.ID()))
	require.Contains(t, metadata, "interfaces")
	assert.JSONEq(t, `[
		{"name":"bridge0","description":"Thunderbolt Bridge","type":"virtual","macaddress":"36:80:7e:c5:c4:00","mtu":1500},
		{"name":"en0","description":"Wi-Fi","type":"wifi","macaddress":"9a:62:9c:bd:37:0a","permanent_macaddress":"bc:d0:74:1e:bd:b7","mtu":1500,
			"builtin":true,"driver":"com.apple.DriverKit-AppleBCMWLAN"},
		{"name":"en4","description":"Ethernet Adapter (en4)","type":"virtual","macaddress":"42:52:1c:e5:c2:ec","mtu":1500,
			"builtin":false,"driver":"com.apple.driver.AppleUSBDeviceNCM","driver_version":"5.0.0"},
		{"name":"en5","description":"Ethernet","type":"ethernet","macaddress":"a4:fc:14:0d:5e:21","mtu":1500,
			"builtin":true,"driver":"com.apple.driver.AppleEthernetAquantiaAqtion","driver_version":"1.0.64"},
		{"name":"en7","description":"USB 10/100/1000 LAN","type":"ethernet","macaddress":"00:e0:4c:68:01:02","mtu":1500,"duplex":"full",
			"builtin":false,"vendor":"Realtek","model":"USB 10/100/1000 LAN","driver":"com.apple.DriverKit-AppleUserECM"},
		{"name":"lo0","type":"loopback","mtu":16384},
		{"name":"utun3","type":"tunnel","mtu":1000},
		{"name":"vlan0","type":"virtual","macaddress":"00:e0:4c:68:01:02","mtu":1500,"duplex":"full","vlan_id":10}
	]`, metadata["interfaces"].(string))
}

// TestInterfacesError tests that the check fails without committing when it cannot list the interfaces.
func TestInterfacesError(t *testing.T) {
	netStats := newFakeNetworkStats()
	netStats.interfaces = nil
	netStats.interfacesErr = errors.New("sysctl failed")
	_, mockSender, err := runCheck(t, netStats, ``)

	assert.Error(t, err)
	mockSender.AssertNotCalled(t, "Commit")
}

// TestTCPStats tests the TCP counters as rates and as monotonic counts.
func TestTCPStats(t *testing.T) {
	_, mockSender, err := runCheck(t, newFakeNetworkStats(), ``)
	require.NoError(t, err)
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.retrans_packs", float64(5009), "", []string(nil))
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.sent_packs", float64(3651535), "", []string(nil))
	mockSender.AssertCalled(t, "Rate", "system.net.tcp.rcv_packs", float64(4807551), "", []string(nil))
	mockSender.AssertNotCalled(t, "MonotonicCount", "system.net.tcp.sent_packs.count", mock.Anything, mock.Anything, mock.Anything)

	_, mockSender, err = runCheck(t, newFakeNetworkStats(), `
collect_rate_metrics: false
collect_count_metrics: true
`)
	require.NoError(t, err)
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.retrans_packs.count", float64(5009), "", []string(nil))
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.sent_packs.count", float64(3651535), "", []string(nil))
	mockSender.AssertCalled(t, "MonotonicCount", "system.net.tcp.rcv_packs.count", float64(4807551), "", []string(nil))
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.sent_packs", mock.Anything, mock.Anything, mock.Anything)
}

// TestTCPStatsUnprivileged tests that the zeroed TCP counters reported to other users than root
// are not submitted.
func TestTCPStatsUnprivileged(t *testing.T) {
	netStats := newFakeNetworkStats()
	netStats.netstat["-s -p tcp"] = netstatTCPStatsUnprivileged
	_, mockSender, err := runCheck(t, netStats, ``)
	require.NoError(t, err)

	for _, metric := range []string{"system.net.tcp.retrans_packs", "system.net.tcp.sent_packs", "system.net.tcp.rcv_packs"} {
		mockSender.AssertNotCalled(t, "Rate", metric, mock.Anything, mock.Anything, mock.Anything)
	}
}

// TestConnectionStates tests the connection counts, combined and per TCP state, and that they
// need collect_connection_state.
func TestConnectionStates(t *testing.T) {
	_, mockSender, err := runCheck(t, newFakeNetworkStats(), ``)
	require.NoError(t, err)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.tcp4.established", mock.Anything, mock.Anything, mock.Anything)

	_, mockSender, err = runCheck(t, newFakeNetworkStats(), `
collect_connection_state: true
`)
	require.NoError(t, err)
	for metric, value := range map[string]float64{
		"system.net.tcp4.established": 1,
		"system.net.tcp4.opening":     1,
		"system.net.tcp4.listening":   1,
		"system.net.tcp4.time_wait":   1,
		"system.net.tcp4.closing":     2,
		"system.net.tcp6.established": 0,
		"system.net.tcp6.opening":     0,
		"system.net.tcp6.listening":   1,
		"system.net.tcp6.time_wait":   0,
		"system.net.tcp6.closing":     0,
		"system.net.udp4.connections": 2,
		"system.net.udp6.connections": 1,
	} {
		mockSender.AssertCalled(t, "Gauge", metric, value, "", []string(nil))
	}

	_, mockSender, err = runCheck(t, newFakeNetworkStats(), `
collect_connection_state: true
combine_connection_states: false
`)
	require.NoError(t, err)
	for metric, value := range map[string]float64{
		"system.net.tcp4.estab":      1,
		"system.net.tcp4.syn_sent":   0,
		"system.net.tcp4.syn_recv":   1,
		"system.net.tcp4.fin_wait_1": 0,
		"system.net.tcp4.fin_wait_2": 1,
		"system.net.tcp4.time_wait":  1,
		"system.net.tcp4.close":      1,
		"system.net.tcp4.close_wait": 0,
		"system.net.tcp4.listen":     1,
		"system.net.tcp4.closing":    0,
		"system.net.tcp6.listen":     1,
		"system.net.tcp6.estab":      0,
	} {
		mockSender.AssertCalled(t, "Gauge", metric, value, "", []string(nil))
	}
}

// TestNetstatError tests that a netstat failure only leaves out the metrics that come from netstat.
func TestNetstatError(t *testing.T) {
	netStats := newFakeNetworkStats()
	netStats.netstatErr = errors.New("netstat timed out")
	_, mockSender, err := runCheck(t, netStats, `
collect_connection_state: true
`)
	require.NoError(t, err)

	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(5000), "", []string{"device:en0"})
	mockSender.AssertNotCalled(t, "Rate", "system.net.tcp.sent_packs", mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.tcp4.established", mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.udp4.connections", mock.Anything, mock.Anything, mock.Anything)
	mockSender.AssertCalled(t, "Commit")
}
