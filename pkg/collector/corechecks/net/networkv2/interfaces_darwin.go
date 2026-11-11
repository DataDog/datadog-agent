// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package networkv2

import (
	"fmt"
	"net"
	"slices"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/collector/corechecks/net/networkv2/nicinfo"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// IFRTYPE_FUNCTIONAL_* values from <net/if.h>
const (
	functionalTypeLoopback      = 1
	functionalTypeWired         = 2
	functionalTypeWifiInfra     = 3
	functionalTypeWifiAWDL      = 4
	functionalTypeCellular      = 5
	functionalTypeIntCoproc     = 6
	functionalTypeCompanionLink = 7
	functionalTypeManagement    = 8
)

// IFM_* values from <net/if_media.h>
const (
	ifmAvalid = 0x1
	ifmActive = 0x2
	ifmNmask  = 0xe0
	ifmEther  = 0x20
	ifmFdx    = 0x00100000
	ifmHdx    = 0x00200000
)

// darwinInterface holds what the kernel reports for one interface.
type darwinInterface struct {
	name           string
	flags          uint32
	ifType         uint8
	functionalType uint32
	macAddress     string
	mtu            uint32
	baudrate       uint64
	// media is nil when the interface does not support SIOCGIFMEDIA
	media  *ifMedia
	vlanID uint16
	nic    nicinfo.Info

	ipackets  uint64
	ierrors   uint64
	iqdrops   uint64
	ibytes    uint64
	opackets  uint64
	oerrors   uint64
	sendDrops uint64
	obytes    uint64
}

type ifMedia struct {
	status int32
	active int32
}

// ifdataGeneral is IFDATA_GENERAL from <net/if_mib.h>
const ifdataGeneral = 1

// ifmibData is struct ifmibdata from <net/if_mib.h>. Its if_data64 is only 4-byte aligned, so it
// is kept as bytes.
type ifmibData struct {
	name      [unix.IFNAMSIZ]byte
	pcount    uint32
	flags     uint32
	sndLen    uint32
	sndMaxlen uint32
	sndDrops  uint32
	filler    [4]uint32
	data      [unsafe.Sizeof(unix.IfData64{})]byte
}

// getInterfaces returns every interface with its counters, functional type, media and VLAN.
func getInterfaces() ([]darwinInterface, error) {
	netIfaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	fd, err := unix.Socket(unix.AF_INET, unix.SOCK_DGRAM, 0)
	if err != nil {
		log.Debugf("Unable to open a socket to query network interfaces: %s", err)
		fd = -1
	} else {
		defer unix.Close(fd)
	}

	ifaces := make([]darwinInterface, 0, len(netIfaces))
	for _, netIface := range netIfaces {
		// The byte counters of the interface list (NET_RT_IFLIST2) are rounded for processes
		// that are not Apple platform binaries, unlike the ones of net.link.generic.ifdata
		buf, err := unix.SysctlRaw("net.link.generic.ifdata", netIface.Index, ifdataGeneral)
		if err != nil {
			log.Debugf("Unable to get the data of interface %s: %s", netIface.Name, err)
			continue
		}
		iface, err := parseIfmibData(buf)
		if err != nil {
			log.Debugf("Unable to parse the data of interface %s: %s", netIface.Name, err)
			continue
		}
		iface.name = netIface.Name
		iface.macAddress = netIface.HardwareAddr.String()
		if fd >= 0 {
			iface.query(fd)
		}
		ifaces = append(ifaces, iface)
	}
	return ifaces, nil
}

const nicInfoRefreshInterval = 10 * time.Minute

// nicInfoCache keeps what IOKit and SystemConfiguration report about the interfaces, which mostly
// changes when an interface appears or disappears.
type nicInfoCache struct {
	get         func() (map[string]nicinfo.Info, error)
	infos       map[string]nicinfo.Info
	names       []string
	lastRefresh time.Time
}

// newNICInfoCache returns an empty cache that gets the interface details with get.
func newNICInfoCache(get func() (map[string]nicinfo.Info, error)) *nicInfoCache {
	return &nicInfoCache{get: get}
}

// enrich sets the details of each interface, refreshing them when the interfaces change or when
// they are older than nicInfoRefreshInterval.
func (c *nicInfoCache) enrich(ifaces []darwinInterface) {
	names := make([]string, 0, len(ifaces))
	for _, iface := range ifaces {
		names = append(names, iface.name)
	}
	slices.Sort(names)
	if c.infos == nil || !slices.Equal(names, c.names) || time.Since(c.lastRefresh) >= nicInfoRefreshInterval {
		// A failed query is retried on the next run, and the previous data is kept until then
		if infos, err := c.get(); err != nil {
			log.Debugf("Keeping the previous network interface details: %s", err)
		} else {
			c.infos = infos
			c.names = names
			c.lastRefresh = time.Now()
		}
	}
	for i := range ifaces {
		ifaces[i].nic = c.infos[ifaces[i].name]
	}
}

// parseIfmibData returns the flags, type, MTU, baud rate and counters of a struct ifmibdata.
func parseIfmibData(buf []byte) (darwinInterface, error) {
	var mib ifmibData
	if len(buf) < int(unsafe.Sizeof(mib)) {
		return darwinInterface{}, fmt.Errorf("got %d bytes, want %d", len(buf), unsafe.Sizeof(mib))
	}
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&mib)), unsafe.Sizeof(mib)), buf)
	var data unix.IfData64
	copy(unsafe.Slice((*byte)(unsafe.Pointer(&data)), unsafe.Sizeof(data)), mib.data[:])

	return darwinInterface{
		// ifmd_flags is the 16-bit if_flags, sign-extended
		flags:     uint32(uint16(mib.flags)),
		ifType:    data.Type,
		mtu:       data.Mtu,
		baudrate:  data.Baudrate,
		ipackets:  data.Ipackets,
		ierrors:   data.Ierrors,
		iqdrops:   data.Iqdrops,
		ibytes:    data.Ibytes,
		opackets:  data.Opackets,
		oerrors:   data.Oerrors,
		sendDrops: uint64(mib.sndDrops),
		obytes:    data.Obytes,
	}, nil
}

// ifreqFunctionalType is struct ifreq with ifr_functional_type.
type ifreqFunctionalType struct {
	name           [unix.IFNAMSIZ]byte
	functionalType uint32
	_              [12]byte
}

// ifMediaReq is struct ifmediareq, which <net/if.h> packs to 4 bytes.
type ifMediaReq struct {
	name    [unix.IFNAMSIZ]byte
	current int32
	mask    int32
	status  int32
	active  int32
	count   int32
	ulist   [8]byte
}

// ifreqData is struct ifreq with ifr_data.
type ifreqData struct {
	name [unix.IFNAMSIZ]byte
	data unsafe.Pointer
	_    [8]byte
}

// vlanReq is struct vlanreq from the XNU <net/if_vlan_var.h>.
type vlanReq struct {
	parent [unix.IFNAMSIZ]byte
	tag    uint16
}

// ioctl calls ioctl(2) with a pointer argument.
func ioctl(fd int, req uint, arg unsafe.Pointer) error {
	//nolint:staticcheck // SA1019: x/sys/unix does not export its libSystem ioctl wrapper for pointer arguments
	if _, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), uintptr(req), uintptr(arg)); errno != 0 {
		return errno
	}
	return nil
}

// query sets the functional type, media and VLAN tag of the interface, leaving the ones that the
// interface does not support unset.
func (i *darwinInterface) query(fd int) {
	ft := ifreqFunctionalType{}
	copy(ft.name[:], i.name)
	if err := ioctl(fd, unix.SIOCGIFFUNCTIONALTYPE, unsafe.Pointer(&ft)); err == nil {
		i.functionalType = ft.functionalType
	} else {
		log.Tracef("SIOCGIFFUNCTIONALTYPE failed on %s: %s", i.name, err)
	}

	media := ifMediaReq{}
	copy(media.name[:], i.name)
	if err := ioctl(fd, unix.SIOCGIFMEDIA, unsafe.Pointer(&media)); err == nil {
		i.media = &ifMedia{status: media.status, active: media.active}
	} else {
		log.Tracef("SIOCGIFMEDIA failed on %s: %s", i.name, err)
	}

	if i.ifType == unix.IFT_L2VLAN {
		vlan := vlanReq{}
		req := ifreqData{data: unsafe.Pointer(&vlan)}
		copy(req.name[:], i.name)
		if err := ioctl(fd, unix.SIOCGIFVLAN, unsafe.Pointer(&req)); err == nil {
			i.vlanID = vlan.tag
		} else {
			log.Tracef("SIOCGIFVLAN failed on %s: %s", i.name, err)
		}
	}
}

// ifaceType classifies the interface as ethernet, wifi, cellular, loopback, tunnel, virtual or other.
func (i darwinInterface) ifaceType() string {
	switch {
	case i.flags&unix.IFF_LOOPBACK != 0 || i.functionalType == functionalTypeLoopback:
		return "loopback"
	case i.ifType == unix.IFT_GIF || i.ifType == unix.IFT_STF || (i.ifType == unix.IFT_OTHER && i.flags&unix.IFF_POINTOPOINT != 0):
		return "tunnel"
	case i.ifType == unix.IFT_BRIDGE || i.ifType == unix.IFT_L2VLAN || i.ifType == unix.IFT_IEEE8023ADLAG:
		return "virtual"
	case i.functionalType == functionalTypeWifiInfra:
		return "wifi"
	case i.functionalType == functionalTypeCellular:
		return "cellular"
	case i.functionalType == functionalTypeWifiAWDL || i.functionalType == functionalTypeIntCoproc ||
		i.functionalType == functionalTypeCompanionLink || i.functionalType == functionalTypeManagement:
		return "virtual"
	// The kernel reports the network interfaces of Virtualization.framework VMs as wired
	case strings.HasPrefix(i.name, "vmenet"):
		return "virtual"
	// The Mac is the USB device of AppleUSBDeviceNCM interfaces, which are not Ethernet adapters
	case strings.HasPrefix(i.nic.DriverClass, "AppleUSBDeviceNCM"):
		return "virtual"
	// Thunderbolt IP ports carry IP over Thunderbolt cables
	case i.nic.DriverClass == "AppleThunderboltIPPort":
		return "other"
	case i.functionalType == functionalTypeWired && i.ifType == unix.IFT_ETHER:
		return "ethernet"
	}
	return "other"
}

// ifaceInfo returns the interface as the gauges and metadata describe it.
func (i darwinInterface) ifaceInfo() ifaceInfo {
	iface := ifaceInfo{
		name:                i.name,
		description:         i.nic.Description,
		ifaceType:           i.ifaceType(),
		macAddress:          i.macAddress,
		permanentMacAddress: i.nic.PermanentMAC,
		mtu:                 i.mtu,
		vlanID:              uint32(i.vlanID),
		builtin:             i.nic.Builtin,
		vendor:              i.nic.Vendor,
		model:               i.nic.Model,
		driver:              i.nic.Driver,
		driverVersion:       i.nic.DriverVersion,
	}

	switch {
	case i.media == nil:
		// Tunnels and the loopback interface have no media, so their flags tell whether they are up
		if i.flags&(unix.IFF_UP|unix.IFF_RUNNING) == unix.IFF_UP|unix.IFF_RUNNING {
			iface.link = linkStateUp
		} else {
			iface.link = linkStateDown
		}
	case i.media.status&ifmAvalid == 0:
		iface.link = linkStateUnknown
	case i.media.status&ifmActive != 0:
		iface.link = linkStateUp
	default:
		iface.link = linkStateDown
	}

	if iface.link == linkStateUp {
		iface.speedMbps = float64(i.baudrate) / 1e6
		if i.media != nil && i.media.active&ifmNmask == ifmEther {
			switch {
			case i.media.active&ifmFdx != 0:
				iface.duplex = "full"
			case i.media.active&ifmHdx != 0:
				iface.duplex = "half"
			}
		}
	}
	return iface
}
