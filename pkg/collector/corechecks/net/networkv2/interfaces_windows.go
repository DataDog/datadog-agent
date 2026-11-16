// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows

package networkv2

import (
	"math"
	"net"
	"slices"
	"sync"
	"time"
	"unsafe"

	"github.com/yusufpapurcu/wmi"
	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// MIB_IF_ROW2.InterfaceAndOperStatusFlags bits
const (
	ifFlagHardwareInterface = 1 << 0
	ifFlagFilterInterface   = 1 << 1
)

// NET_IF_MEDIA_CONNECT_STATE values
const (
	mediaConnectStateConnected    = 1
	mediaConnectStateDisconnected = 2
)

// NET_IF_MEDIA_DUPLEX_STATE values
const (
	mediaDuplexStateHalf = 1
	mediaDuplexStateFull = 2
)

// NDIS_PHYSICAL_MEDIUM values
const (
	ndisPhysicalMediumUnspecified = 0
	ndisPhysicalMediumWirelessLan = 1
	ndisPhysicalMediumWirelessWan = 8
	ndisPhysicalMediumNative80211 = 9
	ndisPhysicalMedium8023        = 14
)

// IANA ifType values missing from golang.org/x/sys/windows
const (
	ifTypeWwanPP  = 243
	ifTypeWwanPP2 = 244
)

// NET_IF_LINK_SPEED_UNKNOWN
const linkSpeedUnknown = math.MaxUint64

const (
	netAdapterRefreshInterval = 10 * time.Minute
	netAdapterFirstLoadWait   = 5 * time.Second
)

// interfaceDetails holds the MIB_IF_ROW2 fields reported for one interface.
type interfaceDetails struct {
	index               uint32
	alias               string
	description         string
	ifType              uint32
	physicalMedium      uint32
	hardware            bool
	mtu                 uint32
	mediaConnectState   uint32
	transmitLinkSpeed   uint64
	macAddress          string
	permanentMacAddress string
}

// getInterfaceDetails returns every interface except NDIS filter interfaces.
func getInterfaceDetails() ([]interfaceDetails, error) {
	var table *windows.MibIfTable2
	if err := windows.GetIfTable2Ex(windows.MibIfTableNormal, &table); err != nil {
		return nil, err
	}
	defer windows.FreeMibTable(unsafe.Pointer(table))

	rows := unsafe.Slice(&table.Table[0], table.NumEntries)
	details := make([]interfaceDetails, 0, len(rows))
	for i := range rows {
		row := &rows[i]
		if row.InterfaceAndOperStatusFlags&ifFlagFilterInterface != 0 {
			continue
		}
		addrLen := min(row.PhysicalAddressLength, windows.IF_MAX_PHYS_ADDRESS_LENGTH)
		details = append(details, interfaceDetails{
			index:               row.InterfaceIndex,
			alias:               windows.UTF16ToString(row.Alias[:]),
			description:         windows.UTF16ToString(row.Description[:]),
			ifType:              row.Type,
			physicalMedium:      row.PhysicalMediumType,
			hardware:            row.InterfaceAndOperStatusFlags&ifFlagHardwareInterface != 0,
			mtu:                 row.Mtu,
			mediaConnectState:   row.MediaConnectState,
			transmitLinkSpeed:   row.TransmitLinkSpeed,
			macAddress:          formatMacAddress(row.PhysicalAddress[:addrLen]),
			permanentMacAddress: formatMacAddress(row.PermanentPhysicalAddress[:addrLen]),
		})
	}
	return details, nil
}

// formatMacAddress returns addr as colon-separated hex bytes, or "" when it is all zeros.
func formatMacAddress(addr []byte) string {
	if !slices.ContainsFunc(addr, func(b byte) bool { return b != 0 }) {
		return ""
	}
	return net.HardwareAddr(addr).String()
}

// ifaceType classifies the interface as ethernet, wifi, cellular, loopback, tunnel, virtual or other.
func (d interfaceDetails) ifaceType() string {
	switch {
	case d.ifType == windows.IF_TYPE_SOFTWARE_LOOPBACK:
		return "loopback"
	case d.ifType == windows.IF_TYPE_TUNNEL:
		return "tunnel"
	case !d.hardware:
		return "virtual"
	case d.physicalMedium == ndisPhysicalMediumNative80211 || d.physicalMedium == ndisPhysicalMediumWirelessLan || d.ifType == windows.IF_TYPE_IEEE80211:
		return "wifi"
	case d.physicalMedium == ndisPhysicalMediumWirelessWan || d.ifType == ifTypeWwanPP || d.ifType == ifTypeWwanPP2:
		return "cellular"
	case d.ifType == windows.IF_TYPE_ETHERNET_CSMACD && (d.physicalMedium == ndisPhysicalMedium8023 || d.physicalMedium == ndisPhysicalMediumUnspecified):
		return "ethernet"
	}
	return "other"
}

// newIfaceInfo merges the MIB_IF_ROW2 fields of an interface with its MSFT_NetAdapter properties.
func newIfaceInfo(details interfaceDetails, adapter msftNetAdapter) ifaceInfo {
	iface := ifaceInfo{
		name:                details.alias,
		description:         details.description,
		ifaceType:           details.ifaceType(),
		macAddress:          details.macAddress,
		permanentMacAddress: details.permanentMacAddress,
		duplex:              adapter.duplex(),
		vlanID:              adapter.VlanID,
		driverProvider:      adapter.DriverProvider,
		driverVersion:       adapter.DriverVersionString,
	}
	// The loopback interface reports an MTU of MaxUint32
	if details.mtu != math.MaxUint32 {
		iface.mtu = details.mtu
	}
	switch details.mediaConnectState {
	case mediaConnectStateConnected:
		iface.link = linkStateUp
		if details.transmitLinkSpeed != linkSpeedUnknown {
			iface.speedMbps = float64(details.transmitLinkSpeed) / 1e6
		}
	case mediaConnectStateDisconnected:
		iface.link = linkStateDown
	}
	return iface
}

// msftNetAdapter holds the MSFT_NetAdapter properties reported for one adapter. Field names
// must match the WMI property names.
type msftNetAdapter struct {
	InterfaceIndex      uint32
	MediaDuplexState    uint32
	VlanID              uint32
	DriverProvider      string
	DriverVersionString string
}

// duplex returns full or half, or "" when the adapter does not report its duplex state.
func (a msftNetAdapter) duplex() string {
	switch a.MediaDuplexState {
	case mediaDuplexStateFull:
		return "full"
	case mediaDuplexStateHalf:
		return "half"
	}
	return ""
}

// queryNetAdapters uses its own SWbemServices, because wmi.Query holds a process-wide lock until
// the query returns, and a WMI query cannot be canceled.
func queryNetAdapters() ([]msftNetAdapter, error) {
	services, err := wmi.InitializeSWbemServices(&wmi.Client{})
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := services.Close(); err != nil {
			log.Debugf("Unable to close the SWbemServices client: %s", err)
		}
	}()
	var adapters []msftNetAdapter
	err = services.Query("SELECT InterfaceIndex, MediaDuplexState, VlanID, DriverProvider, DriverVersionString FROM MSFT_NetAdapter", &adapters, nil, `root\StandardCimv2`)
	return adapters, err
}

// netAdapterCache refreshes MSFT_NetAdapter properties in the background, because a WMI query
// can block for a long time. A query that never returns blocks later refreshes, but not the check.
type netAdapterCache struct {
	query         func() ([]msftNetAdapter, error)
	firstLoadWait time.Duration

	loaded     chan struct{}
	loadedOnce sync.Once

	mu          sync.Mutex
	adapters    map[uint32]msftNetAdapter
	lastRefresh time.Time
	refreshing  bool
	waited      bool
}

// newNetAdapterCache returns an empty cache that gets the adapter properties with query.
func newNetAdapterCache(query func() ([]msftNetAdapter, error)) *netAdapterCache {
	return &netAdapterCache{
		query:         query,
		firstLoadWait: netAdapterFirstLoadWait,
		loaded:        make(chan struct{}),
	}
}

// get returns the adapter properties by interface index, and starts a refresh when they are
// stale. The first call waits up to firstLoadWait for the first refresh, so that the first
// check run already has duplex and VLAN tags; later calls never wait.
func (c *netAdapterCache) get() map[uint32]msftNetAdapter {
	c.mu.Lock()
	if !c.refreshing && time.Since(c.lastRefresh) >= netAdapterRefreshInterval {
		c.refreshing = true
		go c.refresh()
	}
	wait := !c.waited
	c.waited = true
	c.mu.Unlock()

	if wait {
		select {
		case <-c.loaded:
		case <-time.After(c.firstLoadWait):
			log.Debugf("MSFT_NetAdapter query did not complete within %s, reporting interfaces without its properties", c.firstLoadWait)
		}
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.adapters
}

// refresh queries the adapter properties, keeping the previous ones when the query fails, and
// ends the wait of the first get.
func (c *netAdapterCache) refresh() {
	adapters, err := c.query()

	c.mu.Lock()
	c.refreshing = false
	c.lastRefresh = time.Now()
	if err != nil {
		log.Warnf("Unable to query MSFT_NetAdapter, keeping the previous duplex, VLAN and driver data: %s", err)
	} else {
		byIndex := make(map[uint32]msftNetAdapter, len(adapters))
		for _, adapter := range adapters {
			byIndex[adapter.InterfaceIndex] = adapter
		}
		c.adapters = byIndex
	}
	c.mu.Unlock()

	c.loadedOnce.Do(func() { close(c.loaded) })
}
