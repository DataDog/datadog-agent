// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"fmt"
	"strings"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iphelper"
)

// IANA ifType constants
// https://www.iana.org/assignments/ianaiftype-mib/ianaiftype-mib
const (
	ifTypeOther          = 1   // IF_TYPE_OTHER
	ifTypeEthernetCSMACD = 6   // IF_TYPE_ETHERNET_CSMACD
	ifTypeSoftwareLoop   = 24  // IF_TYPE_SOFTWARE_LOOPBACK
	ifTypePPP            = 23  // IF_TYPE_PPP
	ifTypePropVirtual    = 53  // IF_TYPE_PROP_VIRTUAL
	ifTypeWifi           = 71  // IF_TYPE_IEEE80211
	ifTypeTunnel         = 131 // IF_TYPE_TUNNEL
)

// ifTypeToString maps IANA ifType values to human-readable names
var ifTypeToString = map[uint32]string{
	ifTypeOther:          "other",
	ifTypeEthernetCSMACD: "ethernet",
	ifTypePPP:            "ppp",
	ifTypeSoftwareLoop:   "loopback",
	ifTypePropVirtual:    "prop_virtual",
	ifTypeWifi:           "wifi",
	ifTypeTunnel:         "tunnel",
}

// ifTypeName returns a human-readable string for an IANA ifType value
func ifTypeName(ifType uint32) string {
	if name, ok := ifTypeToString[ifType]; ok {
		return name
	}
	return fmt.Sprintf("other_%d", ifType)
}

// InterfaceClassification holds interface metadata and optional VPN classification
type InterfaceClassification struct {
	// Interface metadata (always populated when interface is found)
	InterfaceName string // friendly name, e.g. "Intel Wi-Fi 6 AX201", "WireGuard Tunnel"
	InterfaceType string // human-readable ifType, e.g. "ethernet", "wifi", "prop_virtual"
	IsPhysical    bool   // true if adapter has a MAC address (PhysAddrLen > 0)

	// VPN classification (only populated if interface is identified as VPN)
	IsVPN   bool
	VPNName string // e.g., "GlobalProtect", "Cisco AnyConnect"
	VPNType string // e.g., "prop_virtual", "ppp"
}

// vpnPattern maps a substring to a known VPN product name
type vpnPattern struct {
	pattern string
	name    string
}

// Known VPN adapter name/description patterns (case-insensitive matching)
var vpnPatterns = []vpnPattern{
	// Order matters: more specific patterns must come before generic ones
	{"appgate", "Appgate SDP"},
	{"anyconnect", "Cisco AnyConnect"},
	{"cisco", "Cisco VPN"},
	{"pangp", "GlobalProtect"},
	{"globalprotect", "GlobalProtect"},
	{"palo alto", "GlobalProtect"},
	{"wireguard", "WireGuard"},
	{"wintun", "WireGuard"},
	{"tap-windows", "OpenVPN"},
	{"openvpn", "OpenVPN"},
	{"fortinet", "FortiClient"},
	{"forticlient", "FortiClient"},
	{"pulse secure", "Pulse Secure"},
	{"juniper", "Juniper VPN"},
	{"zscaler", "Zscaler"},
	{"netskope", "Netskope"},
	{"cloudflare", "Cloudflare WARP"},
	{"nordlynx", "NordVPN"},
	{"wan miniport", "Windows VPN"},
	{"surfshark", "Surfshark"},
	{"expressvpn", "ExpressVPN"},
	{"proton", "ProtonVPN"},
}

// cachedInterface stores minimal info about a network interface
type cachedInterface struct {
	ifType      uint32
	name        string // from MibIfRow.Name (UTF-16 friendly name)
	descr       string // from MibIfRow.Descr
	physAddrLen uint32 // MAC address length; 0 means virtual/no physical address
}

// friendlyName returns a human-readable interface name, preferring the
// description (e.g. "WireGuard Tunnel") over the raw device path.
func (ci cachedInterface) friendlyName() string {
	if ci.descr != "" {
		return ci.descr
	}
	return ci.name
}

// VPNClassifier classifies network interfaces as VPN or non-VPN
// by periodically refreshing interface metadata from the OS
type VPNClassifier struct {
	mu      sync.RWMutex
	ifCache map[uint32]cachedInterface // ifIndex -> metadata
	done    chan struct{}
}

// NewVPNClassifier creates a new classifier and starts a background refresh loop
func NewVPNClassifier() *VPNClassifier {
	c := &VPNClassifier{
		ifCache: make(map[uint32]cachedInterface),
		done:    make(chan struct{}),
	}
	log.Infof("vpnclassifier: initializing VPN classifier")
	c.refreshCache()

	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				c.refreshCache()
			case <-c.done:
				return
			}
		}
	}()

	return c
}

// refreshCache queries the OS interface table and rebuilds the cache
func (c *VPNClassifier) refreshCache() {
	table, err := iphelper.GetIFTable()
	if err != nil {
		log.Warnf("vpnclassifier: failed to get interface table: %v", err)
		return
	}

	newCache := make(map[uint32]cachedInterface, len(table))
	for idx, row := range table {
		ci := cachedInterface{
			ifType:      row.Type,
			name:        mibIfRowName(row),
			descr:       mibIfRowDescr(row),
			physAddrLen: row.PhysAddrLen,
		}
		newCache[idx] = ci
		log.Debugf("vpnclassifier: interface idx=%d ifType=%d name=%q descr=%q physAddrLen=%d", idx, ci.ifType, ci.name, ci.descr, ci.physAddrLen)
	}

	log.Infof("vpnclassifier: cached %d interfaces", len(newCache))

	c.mu.Lock()
	c.ifCache = newCache
	c.mu.Unlock()
}

// Classify returns interface metadata and optional VPN classification for the given interface index
func (c *VPNClassifier) Classify(interfaceIndex uint32) InterfaceClassification {
	c.mu.RLock()
	iface, ok := c.ifCache[interfaceIndex]
	c.mu.RUnlock()

	if !ok {
		return InterfaceClassification{}
	}

	// Base interface metadata — always populated
	result := InterfaceClassification{
		InterfaceName: iface.friendlyName(),
		InterfaceType: ifTypeName(iface.ifType),
		IsPhysical:    iface.physAddrLen > 0,
	}

	// VPN classification — only populated when interface is identified as VPN
	combined := strings.ToLower(iface.name + " " + iface.descr)

	switch iface.ifType {
	case ifTypePPP:
		// PPP adapters (WAN Miniport) are almost always VPN on laptops
		vpnName := matchVPNPattern(combined)
		if vpnName == "" {
			vpnName = "Windows VPN"
		}
		result.IsVPN = true
		result.VPNName = vpnName
		result.VPNType = "ppp"

	case ifTypePropVirtual:
		// Proprietary virtual adapters: VPN only if name matches a known pattern
		if vpnName := matchVPNPattern(combined); vpnName != "" {
			result.IsVPN = true
			result.VPNName = vpnName
			result.VPNType = "prop_virtual"
		}

	case ifTypeTunnel:
		// Tunnel adapters: VPN candidate, verify by name
		if vpnName := matchVPNPattern(combined); vpnName != "" {
			result.IsVPN = true
			result.VPNName = vpnName
			result.VPNType = "tunnel"
		}

	case ifTypeEthernetCSMACD:
		// Ethernet adapters: VPN only if name matches or contains tap/tun/vpn
		if vpnName := matchVPNPattern(combined); vpnName != "" {
			result.IsVPN = true
			result.VPNName = vpnName
			result.VPNType = "ethernet_tap"
		} else if strings.Contains(combined, "tap") || strings.Contains(combined, "tun") || strings.Contains(combined, "vpn") {
			result.IsVPN = true
			result.VPNName = "Unknown VPN"
			result.VPNType = "ethernet_tap"
		}
	}

	return result
}

// Close stops the background refresh goroutine
func (c *VPNClassifier) Close() {
	close(c.done)
}

// matchVPNPattern returns the VPN product name if combined matches a known pattern
func matchVPNPattern(combined string) string {
	for _, p := range vpnPatterns {
		if strings.Contains(combined, p.pattern) {
			return p.name
		}
	}
	return ""
}

// mibIfRowName extracts the UTF-16 Name field from a MibIfRow as a Go string
func mibIfRowName(row windows.MibIfRow) string {
	// Name is [256]uint16, find the null terminator
	nameSlice := row.Name[:]
	end := 0
	for i, c := range nameSlice {
		if c == 0 {
			end = i
			break
		}
		if i == len(nameSlice)-1 {
			end = len(nameSlice)
		}
	}
	return string(utf16.Decode(nameSlice[:end]))
}

// mibIfRowDescr extracts the Descr field from a MibIfRow as a Go string
func mibIfRowDescr(row windows.MibIfRow) string {
	length := row.DescrLen
	if length > 256 {
		length = 256
	}
	return string(row.Descr[:length])
}
