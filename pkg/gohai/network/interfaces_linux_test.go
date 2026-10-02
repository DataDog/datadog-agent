// This file is licensed under the MIT License.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2014-present Datadog, Inc.

//go:build linux

package network

import (
	"errors"
	"net"
	"syscall"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/vishvananda/netlink"
)

func mockAddressList(t *testing.T, addresses []netlink.Addr, err error) *int {
	t.Helper()
	original := listAddresses
	t.Cleanup(func() { listAddresses = original })
	calls := 0
	listAddresses = func(link netlink.Link, family int) ([]netlink.Addr, error) {
		require.Nil(t, link)
		require.Equal(t, netlink.FAMILY_ALL, family)
		calls++
		return addresses, err
	}
	return &calls
}

func TestLoadInterfaceAddressesSingleDump(t *testing.T) {
	ifaces := make([]net.Interface, 10000)
	for i := range ifaces {
		ifaces[i].Index = i + 1
	}
	calls := mockAddressList(t, nil, nil)
	addresses, err := loadInterfaceAddresses(ifaces)
	require.NoError(t, err)
	require.Empty(t, addresses)
	require.Equal(t, 1, *calls)
}

func TestCollectInfoLinuxAddressSnapshot(t *testing.T) {
	ifaces := []net.Interface{
		{Index: 42, Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0, 17, 34, 51, 68, 85}},
		{Index: 3, Name: "tap0", Flags: net.FlagUp},
		{Index: 9, Name: "ppp0", Flags: net.FlagUp},
	}
	calls := mockAddressList(t, []netlink.Addr{
		{LinkIndex: 9, IPNet: createIPNetAddr("192.0.2.2/32"), Peer: createIPNetAddr("192.0.2.1/24")},
		{LinkIndex: 42, IPNet: createIPNetAddr("198.51.100.10/24")},
		{LinkIndex: 999, IPNet: createIPNetAddr("203.0.113.1/24")},
		{LinkIndex: 42, IPNet: createIPNetAddr("2001:db8:1::10/64")},
		{LinkIndex: 9, IPNet: createIPNetAddr("2001:db8:2::2/128"), Peer: createIPNetAddr("2001:db8:2::1/64")},
		{LinkIndex: 42, IPNet: createIPNetAddr("198.51.100.20/25")},
		{LinkIndex: 42, IPNet: createIPNetAddr("2001:db8:1::20/96")},
		{LinkIndex: 42},
		{LinkIndex: 42, IPNet: &net.IPNet{}},
	}, nil)
	addresses, err := loadInterfaceAddresses(ifaces)
	require.NoError(t, err)
	require.Equal(t, 1, *calls)
	snapshot := make([]networkInterface, len(ifaces))
	for i := range ifaces {
		snapshot[i] = &realNetworkInterface{iface: ifaces[i], addresses: addresses[ifaces[i].Index]}
	}
	setMockInterfaces(t, snapshot, nil)
	info, err := CollectInfo()
	require.NoError(t, err)
	value, warnings, err := info.AsJSON()
	require.NoError(t, err)
	require.Empty(t, warnings)
	require.Equal(t, map[string]interface{}{
		"ipaddress": "198.51.100.10", "ipaddressv6": "2001:db8:1::10", "macaddress": "00:11:22:33:44:55",
		"interfaces": []interface{}{
			map[string]interface{}{
				"name": "eth0", "macaddress": "00:11:22:33:44:55",
				"ipv4": []string{"198.51.100.10", "198.51.100.20"}, "ipv4-network": "198.51.100.0/25",
				"ipv6": []string{"2001:db8:1::10", "2001:db8:1::20"}, "ipv6-network": "2001:db8:1::/96",
			},
			map[string]interface{}{"name": "tap0", "ipv4": []string{}, "ipv6": []string{}},
			map[string]interface{}{
				"name": "ppp0", "ipv4": []string{"192.0.2.2"}, "ipv4-network": "192.0.2.0/24",
				"ipv6": []string{"2001:db8:2::2"}, "ipv6-network": "2001:db8:2::/64",
			},
		},
	}, value)
}

func TestLoadInterfaceAddressesRejectsPartialDump(t *testing.T) {
	for _, err := range []error{errors.New("address dump failed"), syscall.EINTR} {
		t.Run(err.Error(), func(t *testing.T) {
			mockAddressList(t, []netlink.Addr{{LinkIndex: 1, IPNet: createIPNetAddr("192.0.2.1/24")}}, err)
			addresses, gotErr := loadInterfaceAddresses([]net.Interface{{Index: 1}})
			require.ErrorIs(t, gotErr, err)
			require.Nil(t, addresses)
			info, gotErr := CollectInfo()
			require.ErrorIs(t, gotErr, err)
			require.Nil(t, info)
		})
	}
}
