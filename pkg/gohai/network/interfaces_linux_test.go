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
	"github.com/vishvananda/netlink/nl"
	"golang.org/x/sys/unix"
)

func mockAddressList(t *testing.T, messages [][]byte, err error) *int {
	t.Helper()
	original := listAddresses
	t.Cleanup(func() { listAddresses = original })
	calls := 0
	listAddresses = func() ([][]byte, error) {
		calls++
		return messages, err
	}
	return &calls
}

func TestCollectInfoLinuxAddressSnapshot(t *testing.T) {
	ifaces := []net.Interface{
		{Index: 42, Name: "eth0", Flags: net.FlagUp, HardwareAddr: net.HardwareAddr{0, 17, 34, 51, 68, 85}},
		{Index: 3, Name: "tap0", Flags: net.FlagUp},
		{Index: 9, Name: "ppp0", Flags: net.FlagUp},
	}
	calls := mockAddressList(t, [][]byte{
		addressMessage(9, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("192.0.2.1").To4()), nl.NewRtAttr(unix.IFA_LOCAL, net.ParseIP("192.0.2.2").To4())),
		addressMessage(42, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("198.51.100.10").To4())),
		addressMessage(999, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("203.0.113.1").To4())),
		addressMessage(42, unix.AF_INET6, 64, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("2001:db8:1::10").To16())),
		addressMessage(9, unix.AF_INET6, 64, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("2001:db8:2::1").To16()), nl.NewRtAttr(unix.IFA_LOCAL, net.ParseIP("2001:db8:2::2").To16())),
		addressMessage(42, unix.AF_INET, 25, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("198.51.100.20").To4())),
		addressMessage(42, unix.AF_INET6, 96, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("2001:db8:1::20").To16())),
		addressMessage(42, unix.AF_INET, 24),
		addressMessage(42, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, nil)),
	}, nil)
	snapshot, err := loadInterfaces(ifaces)
	require.NoError(t, err)
	require.Equal(t, 1, *calls)
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

func TestLoadInterfacesRejectsPartialDump(t *testing.T) {
	for _, err := range []error{errors.New("address dump failed"), syscall.EINTR} {
		t.Run(err.Error(), func(t *testing.T) {
			mockAddressList(t, [][]byte{addressMessage(1, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("192.0.2.1").To4()))}, err)
			ifaces, gotErr := loadInterfaces([]net.Interface{{Index: 1}})
			require.ErrorIs(t, gotErr, err)
			require.Nil(t, ifaces)
			info, gotErr := CollectInfo()
			require.ErrorIs(t, gotErr, err)
			require.Nil(t, info)
		})
	}
}

func addressMessage(index, family int, prefix uint8, attrs ...*nl.RtAttr) []byte {
	message := nl.NewIfAddrmsg(family)
	message.Index = uint32(index)
	message.Prefixlen = prefix
	data := message.Serialize()
	for _, attr := range attrs {
		data = append(data, attr.Serialize()...)
	}
	return data
}

func TestParseAddress(t *testing.T) {
	v4 := nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("192.0.2.1").To4())
	local4 := nl.NewRtAttr(unix.IFA_LOCAL, net.ParseIP("192.0.2.2").To4())
	v6 := nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("2001:db8::1").To16())
	local6 := nl.NewRtAttr(unix.IFA_LOCAL, net.ParseIP("2001:db8::2").To16())
	for _, test := range []struct {
		name    string
		message []byte
		want    string
	}{
		{"IPv4", addressMessage(42, unix.AF_INET, 24, v4), "192.0.2.1/24"},
		{"IPv4 peer", addressMessage(42, unix.AF_INET, 24, v4, local4), "192.0.2.2/24"},
		{"local before peer", addressMessage(42, unix.AF_INET, 24, local4, v4), "192.0.2.2/24"},
		{"IPv6", addressMessage(42, unix.AF_INET6, 64, v6), "2001:db8::1/64"},
		{"IPv6 peer", addressMessage(42, unix.AF_INET6, 64, v6, local6), "2001:db8::2/64"},
		{"no address", addressMessage(42, unix.AF_INET, 24), ""},
		{"unsupported family", addressMessage(42, unix.AF_UNSPEC, 24, v4), ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			index, addr, err := parseAddress(test.message)
			require.NoError(t, err)
			require.Equal(t, 42, index)
			if test.want == "" {
				require.Nil(t, addr)
			} else {
				require.NotNil(t, addr)
				require.Equal(t, test.want, addr.String())
			}
		})
	}
}

func TestLoadInterfacesRejectsMalformedMessages(t *testing.T) {
	for _, message := range [][]byte{
		nil,
		append(addressMessage(42, unix.AF_INET, 24), 2, 0, 1, 0),
		addressMessage(42, unix.AF_INET, 33, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("192.0.2.1").To4())),
		addressMessage(42, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("2001:db8::1").To16())),
	} {
		valid := addressMessage(42, unix.AF_INET, 24, nl.NewRtAttr(unix.IFA_ADDRESS, net.ParseIP("192.0.2.1").To4()))
		mockAddressList(t, [][]byte{valid, message}, nil)
		ifaces, err := loadInterfaces([]net.Interface{{Index: 42}})
		require.Error(t, err)
		require.Nil(t, ifaces)
	}
}
