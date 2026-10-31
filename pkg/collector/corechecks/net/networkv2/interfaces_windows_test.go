// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package networkv2

import (
	"errors"
	"math"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/shirou/gopsutil/v4/net"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/comp/core/autodiscovery/integration"
	inventorychecksmock "github.com/DataDog/datadog-agent/comp/metadata/inventorychecks/mock"
	"github.com/DataDog/datadog-agent/pkg/aggregator/mocksender"
	"github.com/DataDog/datadog-agent/pkg/collector/check"
	core "github.com/DataDog/datadog-agent/pkg/collector/corechecks"
)

// newInterfacesNetworkStats returns interfaces as a laptop with a wired adapter, Wi-Fi, an
// unplugged USB Ethernet adapter, a Hyper-V switch and the loopback would report them.
func newInterfacesNetworkStats() *fakeNetworkStats {
	return &fakeNetworkStats{
		counterStats: []net.IOCountersStat{
			{Name: "Ethernet"},
			{Name: "Wi-Fi"},
			{Name: "Ethernet 2"},
			{Name: "vEthernet (Default Switch)"},
			{Name: "Loopback Pseudo-Interface 1"},
		},
		tcp4Stats: &mibTCPStats{},
		tcp6Stats: &mibTCPStats{},
		interfaceDetails: []interfaceDetails{
			{
				index:               5,
				alias:               "Ethernet",
				description:         "Intel(R) Ethernet Connection (16) I219-LM",
				ifType:              windows.IF_TYPE_ETHERNET_CSMACD,
				physicalMedium:      ndisPhysicalMedium8023,
				hardware:            true,
				mtu:                 1500,
				mediaConnectState:   mediaConnectStateConnected,
				transmitLinkSpeed:   1_000_000_000,
				macAddress:          "00:15:5d:01:02:03",
				permanentMacAddress: "00:15:5d:01:02:03",
			},
			{
				index:               7,
				alias:               "Wi-Fi",
				description:         "Intel(R) Wi-Fi 6E AX211 160MHz",
				ifType:              windows.IF_TYPE_IEEE80211,
				physicalMedium:      ndisPhysicalMediumNative80211,
				hardware:            true,
				mtu:                 1500,
				mediaConnectState:   mediaConnectStateConnected,
				transmitLinkSpeed:   866_700_000,
				macAddress:          "9a:62:9c:bd:37:0a",
				permanentMacAddress: "bc:d0:74:1e:bd:b7",
			},
			{
				index:               12,
				alias:               "Ethernet 2",
				description:         "Realtek USB GbE Family Controller",
				ifType:              windows.IF_TYPE_ETHERNET_CSMACD,
				physicalMedium:      ndisPhysicalMedium8023,
				hardware:            true,
				mtu:                 1500,
				mediaConnectState:   mediaConnectStateDisconnected,
				transmitLinkSpeed:   linkSpeedUnknown,
				macAddress:          "42:52:1c:e5:c2:ec",
				permanentMacAddress: "42:52:1c:e5:c2:ec",
			},
			{
				index:             20,
				alias:             "vEthernet (Default Switch)",
				description:       "Hyper-V Virtual Ethernet Adapter",
				ifType:            windows.IF_TYPE_ETHERNET_CSMACD,
				physicalMedium:    ndisPhysicalMediumUnspecified,
				mtu:               1500,
				mediaConnectState: mediaConnectStateConnected,
				transmitLinkSpeed: 10_000_000_000,
				macAddress:        "00:15:5d:aa:bb:cc",
			},
			{
				index:             1,
				alias:             "Loopback Pseudo-Interface 1",
				description:       "Software Loopback Interface 1",
				ifType:            windows.IF_TYPE_SOFTWARE_LOOPBACK,
				mtu:               math.MaxUint32,
				mediaConnectState: mediaConnectStateConnected,
				transmitLinkSpeed: 1_073_741_824_000,
			},
			{
				// Not reported by gopsutil, so the check ignores it
				index:             9,
				alias:             "Bluetooth Network Connection",
				ifType:            windows.IF_TYPE_ETHERNET_CSMACD,
				physicalMedium:    10,
				hardware:          true,
				mtu:               1500,
				mediaConnectState: mediaConnectStateDisconnected,
			},
		},
		netAdapters: map[uint32]msftNetAdapter{
			5:  {InterfaceIndex: 5, MediaDuplexState: mediaDuplexStateFull, VlanID: 10, DriverProvider: "Intel", DriverVersionString: "12.19.2.45"},
			7:  {InterfaceIndex: 7, DriverProvider: "Intel", DriverVersionString: "23.60.0.10"},
			12: {InterfaceIndex: 12, DriverProvider: "Realtek", DriverVersionString: "10.68.815.2024"},
		},
	}
}

// runInterfacesCheck configures and runs a check on netStats with a sender that accepts every metric.
func runInterfacesCheck(t *testing.T, netStats *fakeNetworkStats, rawInstanceConfig string) (*NetworkCheck, *mocksender.MockSender) {
	networkCheck := createTestNetworkCheck(netStats)
	networkCheck.CheckBase = core.NewCheckBase(CheckName)

	mockSender := mocksender.NewMockSender(t, networkCheck.ID())
	err := networkCheck.Configure(mockSender.GetSenderManager(), integration.FakeConfigHash, []byte(rawInstanceConfig), []byte(``), "test", "provider")
	require.NoError(t, err)

	mockSender.On("Gauge", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Rate", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("MonotonicCount", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return()
	mockSender.On("Commit").Return()

	require.NoError(t, networkCheck.Run())
	return networkCheck, mockSender
}

// TestInterfaceDetailMetrics tests the interface gauges of each kind of interface, and that
// excluded interfaces get none.
func TestInterfaceDetailMetrics(t *testing.T) {
	_, mockSender := runInterfacesCheck(t, newInterfacesNetworkStats(), `
excluded_interfaces:
  - vEthernet (Default Switch)
`)

	ethernetTags := []string{"iface:Ethernet", "iface_type:ethernet", "duplex:full", "vlan_id:10"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(1500), "", ethernetTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", ethernetTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.speed", float64(1000), "", ethernetTags)

	wifiTags := []string{"iface:Wi-Fi", "iface_type:wifi"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(1500), "", wifiTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", wifiTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", wifiTags)

	unpluggedTags := []string{"iface:Ethernet 2", "iface_type:ethernet"}
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.mtu", float64(1500), "", unpluggedTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(0), "", unpluggedTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", unpluggedTags)

	loopbackTags := []string{"iface:Loopback Pseudo-Interface 1", "iface_type:loopback"}
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.mtu", mock.Anything, "", loopbackTags)
	mockSender.AssertCalled(t, "Gauge", "system.net.iface.up", float64(1), "", loopbackTags)
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.speed", mock.Anything, "", loopbackTags)

	for _, excluded := range [][]string{
		{"iface:vEthernet (Default Switch)", "iface_type:virtual"},
		{"iface:Bluetooth Network Connection", "iface_type:other"},
	} {
		mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.mtu", mock.Anything, "", excluded)
		mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.up", mock.Anything, "", excluded)
	}

	// The traffic rates keep their existing tags
	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(0), "", []string{"device:Ethernet"})
}

// TestInterfacesMetadata tests the "interfaces" check metadata, which merges GetIfTable2Ex with
// MSFT_NetAdapter.
func TestInterfacesMetadata(t *testing.T) {
	invChecks := inventorychecksmock.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	defer check.ReleaseContext()

	networkCheck, _ := runInterfacesCheck(t, newInterfacesNetworkStats(), ``)

	metadata := invChecks.GetInstanceMetadata(string(networkCheck.ID()))
	require.Contains(t, metadata, "interfaces")
	assert.JSONEq(t, `[
		{"name":"Ethernet","description":"Intel(R) Ethernet Connection (16) I219-LM","type":"ethernet","macaddress":"00:15:5d:01:02:03","mtu":1500,"duplex":"full","vlan_id":10,"driver_provider":"Intel","driver_version":"12.19.2.45"},
		{"name":"Ethernet 2","description":"Realtek USB GbE Family Controller","type":"ethernet","macaddress":"42:52:1c:e5:c2:ec","mtu":1500,"driver_provider":"Realtek","driver_version":"10.68.815.2024"},
		{"name":"Loopback Pseudo-Interface 1","description":"Software Loopback Interface 1","type":"loopback"},
		{"name":"Wi-Fi","description":"Intel(R) Wi-Fi 6E AX211 160MHz","type":"wifi","macaddress":"9a:62:9c:bd:37:0a","permanent_macaddress":"bc:d0:74:1e:bd:b7","mtu":1500,"driver_provider":"Intel","driver_version":"23.60.0.10"},
		{"name":"vEthernet (Default Switch)","description":"Hyper-V Virtual Ethernet Adapter","type":"virtual","macaddress":"00:15:5d:aa:bb:cc","mtu":1500}
	]`, metadata["interfaces"].(string))
}

// TestInterfaceDetailsError tests that the check keeps its rates when GetIfTable2Ex fails, and
// leaves the gauges, WMI and metadata out.
func TestInterfaceDetailsError(t *testing.T) {
	invChecks := inventorychecksmock.NewMock().Comp
	check.InitializeInventoryChecksContext(invChecks)
	defer check.ReleaseContext()

	netStats := newInterfacesNetworkStats()
	netStats.interfaceDetails = nil
	netStats.interfaceDetailsError = errors.New("GetIfTable2Ex failed")
	networkCheck, mockSender := runInterfacesCheck(t, netStats, ``)

	mockSender.AssertCalled(t, "Rate", "system.net.bytes_rcvd", float64(0), "", []string{"device:Ethernet"})
	mockSender.AssertNotCalled(t, "Gauge", "system.net.iface.up", mock.Anything, mock.Anything, mock.Anything)
	assert.Zero(t, netStats.netAdaptersCalls)
	// A failed lookup must not replace the last reported interfaces with an empty list
	assert.NotContains(t, invChecks.GetInstanceMetadata(string(networkCheck.ID())), "interfaces")
}

// TestIfaceType tests the iface_type of each kind of Windows interface.
func TestIfaceType(t *testing.T) {
	tests := []struct {
		name    string
		details interfaceDetails
		want    string
	}{
		{"loopback", interfaceDetails{ifType: windows.IF_TYPE_SOFTWARE_LOOPBACK}, "loopback"},
		{"teredo tunnel", interfaceDetails{ifType: windows.IF_TYPE_TUNNEL}, "tunnel"},
		{"hyper-v switch", interfaceDetails{ifType: windows.IF_TYPE_ETHERNET_CSMACD}, "virtual"},
		{"wi-fi direct", interfaceDetails{ifType: windows.IF_TYPE_IEEE80211, physicalMedium: ndisPhysicalMediumNative80211}, "virtual"},
		{"wi-fi", interfaceDetails{ifType: windows.IF_TYPE_IEEE80211, physicalMedium: ndisPhysicalMediumNative80211, hardware: true}, "wifi"},
		{"mobile broadband", interfaceDetails{ifType: ifTypeWwanPP, physicalMedium: ndisPhysicalMediumWirelessWan, hardware: true}, "cellular"},
		{"ethernet", interfaceDetails{ifType: windows.IF_TYPE_ETHERNET_CSMACD, physicalMedium: ndisPhysicalMedium8023, hardware: true}, "ethernet"},
		{"ethernet without medium", interfaceDetails{ifType: windows.IF_TYPE_ETHERNET_CSMACD, hardware: true}, "ethernet"},
		{"bluetooth pan", interfaceDetails{ifType: windows.IF_TYPE_ETHERNET_CSMACD, physicalMedium: 10, hardware: true}, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.details.ifaceType())
		})
	}
}

// TestFormatMacAddress tests that missing and all-zero addresses are left out.
func TestFormatMacAddress(t *testing.T) {
	assert.Equal(t, "", formatMacAddress(nil))
	assert.Equal(t, "", formatMacAddress(make([]byte, 6)))
	assert.Equal(t, "42:52:1c:e5:c2:ec", formatMacAddress([]byte{0x42, 0x52, 0x1c, 0xe5, 0xc2, 0xec}))
}

// TestNetAdapterCache tests when the MSFT_NetAdapter cache refreshes and waits.
func TestNetAdapterCache(t *testing.T) {
	t.Run("first call waits for the first refresh", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			cache := newNetAdapterCache(func() ([]msftNetAdapter, error) {
				return []msftNetAdapter{{InterfaceIndex: 5, MediaDuplexState: mediaDuplexStateFull}}, nil
			})
			start := time.Now()
			assert.Equal(t, map[uint32]msftNetAdapter{5: {InterfaceIndex: 5, MediaDuplexState: mediaDuplexStateFull}}, cache.get())
			assert.Zero(t, time.Since(start), "a query that returns does not wait for the timeout")
		})
	})

	t.Run("refreshes only when stale and keeps the data on error", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			var calls atomic.Int32
			cache := newNetAdapterCache(func() ([]msftNetAdapter, error) {
				if calls.Add(1) == 1 {
					return []msftNetAdapter{{InterfaceIndex: 5}}, nil
				}
				return nil, errors.New("WMI query failed")
			})
			want := map[uint32]msftNetAdapter{5: {InterfaceIndex: 5}}
			assert.Equal(t, want, cache.get())
			assert.Equal(t, want, cache.get())
			assert.EqualValues(t, 1, calls.Load())

			time.Sleep(netAdapterRefreshInterval)
			cache.get()
			synctest.Wait()
			assert.EqualValues(t, 2, calls.Load())
			assert.Equal(t, want, cache.get())
			assert.EqualValues(t, 2, calls.Load(), "a failed query is not retried before the next interval")
		})
	})

	t.Run("a blocked query only delays the first call", func(t *testing.T) {
		synctest.Test(t, func(t *testing.T) {
			release := make(chan struct{})
			var calls atomic.Int32
			cache := newNetAdapterCache(func() ([]msftNetAdapter, error) {
				calls.Add(1)
				<-release
				return []msftNetAdapter{{InterfaceIndex: 5}}, nil
			})
			start := time.Now()
			assert.Empty(t, cache.get())
			assert.Equal(t, netAdapterFirstLoadWait, time.Since(start))

			start = time.Now()
			assert.Empty(t, cache.get())
			assert.Zero(t, time.Since(start), "later calls do not wait")

			close(release)
			synctest.Wait()
			assert.Len(t, cache.get(), 1)
			assert.EqualValues(t, 1, calls.Load())
		})
	})
}
