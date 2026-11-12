// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package networkv2

import (
	"errors"
	"slices"
	"testing"
	"time"
	"unsafe"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/networkv2/nicinfo"
	"github.com/DataDog/datadog-agent/pkg/util/pointer"
)

// TestParseIfmibData tests the layout of struct ifmibdata, and that a short buffer is an error.
func TestParseIfmibData(t *testing.T) {
	// sizeof(struct ifmibdata), with its if_data64 at offset 52
	require.EqualValues(t, 180, unsafe.Sizeof(ifmibData{}))

	data := unix.IfData64{
		Type: unix.IFT_ETHER, Mtu: 1500, Baudrate: 97_110_000,
		Ipackets: 50, Ierrors: 1, Iqdrops: 2, Ibytes: 58_203_059_974, Opackets: 40, Oerrors: 3, Obytes: 29_206_748_559,
	}
	mib := ifmibData{flags: 0xffff0000 | unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING | unix.IFF_MULTICAST, sndDrops: 6}
	copy(mib.name[:], "en0")
	copy(mib.data[:], unsafe.Slice((*byte)(unsafe.Pointer(&data)), unsafe.Sizeof(data)))
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&mib)), unsafe.Sizeof(mib))

	iface, err := parseIfmibData(buf)
	require.NoError(t, err)
	assert.Equal(t, darwinInterface{
		flags: unix.IFF_UP | unix.IFF_BROADCAST | unix.IFF_RUNNING | unix.IFF_MULTICAST, ifType: unix.IFT_ETHER, mtu: 1500, baudrate: 97_110_000,
		ipackets: 50, ierrors: 1, iqdrops: 2, ibytes: 58_203_059_974, opackets: 40, oerrors: 3, sendDrops: 6, obytes: 29_206_748_559,
	}, iface)

	_, err = parseIfmibData(buf[:100])
	assert.Error(t, err)
}

// TestIoctlRequestSizes tests that the ioctl structs have the sizes that their requests encode.
func TestIoctlRequestSizes(t *testing.T) {
	// The size of the argument is encoded in bits 16 to 28 of the request
	size := func(req uint) uintptr { return uintptr(req >> 16 & 0x1fff) }
	assert.Equal(t, size(unix.SIOCGIFFUNCTIONALTYPE), unsafe.Sizeof(ifreqFunctionalType{}))
	assert.Equal(t, size(unix.SIOCGIFMEDIA), unsafe.Sizeof(ifMediaReq{}))
	assert.Equal(t, size(unix.SIOCGIFVLAN), unsafe.Sizeof(ifreqData{}))
}

// TestGetInterfaces reads the interfaces of the host, and tests its loopback interface.
func TestGetInterfaces(t *testing.T) {
	ifaces, err := defaultNetworkStats{nics: newNICInfoCache(nicinfo.Get)}.Interfaces()
	require.NoError(t, err)
	for _, iface := range ifaces {
		t.Logf("%+v %+v", iface, newNicMetadata(iface.ifaceInfo()))
	}

	i := slices.IndexFunc(ifaces, func(iface darwinInterface) bool { return iface.name == "lo0" })
	require.NotEqual(t, -1, i, "lo0 not found")
	lo := ifaces[i]
	assert.EqualValues(t, functionalTypeLoopback, lo.functionalType)
	assert.Equal(t, "loopback", lo.ifaceType())
	assert.NotZero(t, lo.mtu)
	assert.Equal(t, linkStateUp, lo.ifaceInfo().link)
}

// TestDarwinIfaceType tests the iface_type of each kind of macOS interface.
func TestDarwinIfaceType(t *testing.T) {
	tests := []struct {
		name  string
		iface darwinInterface
		want  string
	}{
		{"loopback", darwinInterface{name: "lo0", flags: unix.IFF_LOOPBACK, ifType: unix.IFT_LOOP, functionalType: functionalTypeLoopback}, "loopback"},
		{"gif", darwinInterface{name: "gif0", flags: unix.IFF_POINTOPOINT, ifType: unix.IFT_GIF}, "tunnel"},
		{"stf", darwinInterface{name: "stf0", ifType: unix.IFT_STF}, "tunnel"},
		{"utun", darwinInterface{name: "utun0", flags: unix.IFF_POINTOPOINT, ifType: unix.IFT_OTHER}, "tunnel"},
		{"usb ethernet", darwinInterface{name: "en4", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired}, "ethernet"},
		{"thunderbolt", darwinInterface{
			name: "en1", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired,
			nic: nicinfo.Info{DriverClass: "AppleThunderboltIPPort"},
		}, "other"},
		{"thunderbolt bridge", darwinInterface{name: "bridge0", ifType: unix.IFT_BRIDGE, functionalType: functionalTypeWired}, "virtual"},
		{"vlan", darwinInterface{name: "vlan0", ifType: unix.IFT_L2VLAN, functionalType: functionalTypeWired}, "virtual"},
		{"link aggregation", darwinInterface{name: "bond0", ifType: unix.IFT_IEEE8023ADLAG, functionalType: functionalTypeWired}, "virtual"},
		{"wi-fi", darwinInterface{name: "en0", ifType: unix.IFT_ETHER, functionalType: functionalTypeWifiInfra}, "wifi"},
		{"awdl", darwinInterface{name: "awdl0", ifType: unix.IFT_ETHER, functionalType: functionalTypeWifiAWDL}, "virtual"},
		{"cellular", darwinInterface{name: "pdp_ip0", ifType: unix.IFT_CELLULAR, functionalType: functionalTypeCellular}, "cellular"},
		{"vm", darwinInterface{name: "vmenet0", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired}, "virtual"},
		{"usb device mode", darwinInterface{
			name: "en5", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired,
			nic: nicinfo.Info{DriverClass: "AppleUSBDeviceNCMData"},
		}, "virtual"},
		{"unknown ethernet", darwinInterface{name: "feth0", ifType: unix.IFT_ETHER}, "other"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.iface.ifaceType())
		})
	}
}

// TestDarwinIfaceInfo tests the link state, speed, duplex and IOKit details of macOS interfaces.
func TestDarwinIfaceInfo(t *testing.T) {
	tests := []struct {
		name  string
		iface darwinInterface
		want  ifaceInfo
	}{
		{
			name: "ethernet with link",
			iface: darwinInterface{
				name: "en7", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired, macAddress: "00:e0:4c:68:01:02",
				mtu: 1500, baudrate: 1_000_000_000, media: &ifMedia{status: ifmAvalid | ifmActive, active: ifmEther | ifmFdx | 16},
			},
			want: ifaceInfo{name: "en7", ifaceType: "ethernet", macAddress: "00:e0:4c:68:01:02", mtu: 1500, link: linkStateUp, speedMbps: 1000, duplex: "full"},
		},
		{
			name: "iokit details",
			iface: darwinInterface{
				name: "en7", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired, macAddress: "02:00:00:00:00:01",
				media: &ifMedia{status: ifmAvalid, active: ifmEther},
				nic: nicinfo.Info{
					Description: "USB 10/100/1000 LAN", PermanentMAC: "00:e0:4c:68:01:02", Builtin: pointer.Ptr(false),
					Vendor: "Realtek", Model: "USB 10/100/1000 LAN", DriverClass: "AppleUserECM",
					Driver: "com.apple.DriverKit-AppleUserECM", DriverVersion: "1.0",
				},
			},
			want: ifaceInfo{
				name: "en7", description: "USB 10/100/1000 LAN", ifaceType: "ethernet", macAddress: "02:00:00:00:00:01",
				permanentMacAddress: "00:e0:4c:68:01:02", link: linkStateDown, builtin: pointer.Ptr(false),
				vendor: "Realtek", model: "USB 10/100/1000 LAN", driver: "com.apple.DriverKit-AppleUserECM", driverVersion: "1.0",
			},
		},
		{
			name: "half duplex",
			iface: darwinInterface{
				name: "en8", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired,
				baudrate: 10_000_000, media: &ifMedia{status: ifmAvalid | ifmActive, active: ifmEther | ifmHdx | 3},
			},
			want: ifaceInfo{name: "en8", ifaceType: "ethernet", link: linkStateUp, speedMbps: 10, duplex: "half"},
		},
		{
			name: "unplugged ethernet",
			iface: darwinInterface{
				name: "en1", ifType: unix.IFT_ETHER, functionalType: functionalTypeWired,
				mtu: 1500, media: &ifMedia{status: ifmAvalid, active: ifmEther | ifmFdx},
			},
			want: ifaceInfo{name: "en1", ifaceType: "ethernet", mtu: 1500, link: linkStateDown},
		},
		{
			name: "wi-fi has no duplex",
			iface: darwinInterface{
				name: "en0", ifType: unix.IFT_ETHER, functionalType: functionalTypeWifiInfra,
				baudrate: 866_700_000, media: &ifMedia{status: ifmAvalid | ifmActive, active: 0x80},
			},
			want: ifaceInfo{name: "en0", ifaceType: "wifi", link: linkStateUp, speedMbps: 866.7},
		},
		{
			name:  "media status not valid",
			iface: darwinInterface{name: "llw0", ifType: unix.IFT_ETHER, functionalType: functionalTypeWifiInfra, media: &ifMedia{active: 0x22}},
			want:  ifaceInfo{name: "llw0", ifaceType: "wifi", link: linkStateUnknown},
		},
		{
			name:  "running tunnel",
			iface: darwinInterface{name: "utun0", flags: unix.IFF_UP | unix.IFF_POINTOPOINT | unix.IFF_RUNNING, ifType: unix.IFT_OTHER, mtu: 1380},
			want:  ifaceInfo{name: "utun0", ifaceType: "tunnel", mtu: 1380, link: linkStateUp},
		},
		{
			name:  "down tunnel",
			iface: darwinInterface{name: "gif0", flags: unix.IFF_POINTOPOINT, ifType: unix.IFT_GIF, mtu: 1280},
			want:  ifaceInfo{name: "gif0", ifaceType: "tunnel", mtu: 1280, link: linkStateDown},
		},
		{
			name: "vlan",
			iface: darwinInterface{
				name: "vlan0", ifType: unix.IFT_L2VLAN, functionalType: functionalTypeWired, vlanID: 10,
				baudrate: 1_000_000_000, media: &ifMedia{status: ifmAvalid | ifmActive, active: ifmEther | ifmFdx},
			},
			want: ifaceInfo{name: "vlan0", ifaceType: "virtual", link: linkStateUp, speedMbps: 1000, duplex: "full", vlanID: 10},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, tt.iface.ifaceInfo())
		})
	}
}

// TestNICInfoCache tests when the IOKit details are refreshed, and that a failure keeps them.
func TestNICInfoCache(t *testing.T) {
	calls := 0
	var getErr error
	cache := newNICInfoCache(func() (map[string]nicinfo.Info, error) {
		calls++
		if getErr != nil {
			return nil, getErr
		}
		return map[string]nicinfo.Info{"en0": {Description: "Wi-Fi"}, "en7": {Description: "USB 10/100/1000 LAN"}}, nil
	})
	enrich := func(names ...string) []darwinInterface {
		ifaces := make([]darwinInterface, 0, len(names))
		for _, name := range names {
			ifaces = append(ifaces, darwinInterface{name: name})
		}
		cache.enrich(ifaces)
		return ifaces
	}

	ifaces := enrich("en0", "utun0")
	assert.Equal(t, 1, calls)
	assert.Equal(t, "Wi-Fi", ifaces[0].nic.Description)
	assert.Equal(t, nicinfo.Info{}, ifaces[1].nic)

	ifaces = enrich("utun0", "en0")
	assert.Equal(t, 1, calls, "same interfaces in another order")
	assert.Equal(t, "Wi-Fi", ifaces[1].nic.Description)

	ifaces = enrich("en0", "en7", "utun0")
	assert.Equal(t, 2, calls, "new interface")
	assert.Equal(t, "USB 10/100/1000 LAN", ifaces[1].nic.Description)

	cache.lastRefresh = time.Now().Add(-nicInfoRefreshInterval)
	enrich("en0", "en7", "utun0")
	assert.Equal(t, 3, calls, "stale")

	getErr = errors.New("IOServiceGetMatchingServices failed")
	ifaces = enrich("en0", "en7", "en8", "utun0")
	assert.Equal(t, 4, calls, "new interface")
	assert.Equal(t, "Wi-Fi", ifaces[0].nic.Description, "previous data kept after a failure")
	enrich("en0", "en7", "en8", "utun0")
	assert.Equal(t, 5, calls, "failure retried")

	getErr = nil
	enrich("en0", "en7", "en8", "utun0")
	ifaces = enrich("en0", "en7", "en8", "utun0")
	assert.Equal(t, 6, calls, "no retry after a success")
	assert.Equal(t, "USB 10/100/1000 LAN", ifaces[1].nic.Description)
}
