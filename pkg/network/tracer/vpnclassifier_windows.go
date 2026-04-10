// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
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
	ifTypeEthernetCSMACD = 6   // IF_TYPE_ETHERNET_CSMACD
	ifTypePPP            = 23  // IF_TYPE_PPP
	ifTypePropVirtual    = 53  // IF_TYPE_PROP_VIRTUAL
	ifTypeTunnel         = 131 // IF_TYPE_TUNNEL
)

// VPNClassification holds the result of classifying a network interface
type VPNClassification struct {
	IsVPN     bool
	VPNName   string // e.g., "GlobalProtect", "Cisco AnyConnect"
	VPNType   string // e.g., "prop_virtual", "ppp"
	Interface string // interface friendly name
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
	ifType uint32
	name   string // from MibIfRow.Name (UTF-16 friendly name)
	descr  string // from MibIfRow.Descr
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
			ifType: row.Type,
			name:   mibIfRowName(row),
			descr:  mibIfRowDescr(row),
		}
		newCache[idx] = ci
		log.Debugf("vpnclassifier: interface idx=%d ifType=%d name=%q descr=%q", idx, ci.ifType, ci.name, ci.descr)
	}

	log.Infof("vpnclassifier: cached %d interfaces", len(newCache))

	c.mu.Lock()
	c.ifCache = newCache
	c.mu.Unlock()
}

// Classify determines whether the interface at the given index is a VPN
func (c *VPNClassifier) Classify(interfaceIndex uint32) VPNClassification {
	c.mu.RLock()
	iface, ok := c.ifCache[interfaceIndex]
	c.mu.RUnlock()

	if !ok {
		return VPNClassification{}
	}

	combined := strings.ToLower(iface.name + " " + iface.descr)

	switch iface.ifType {
	case ifTypePPP:
		// PPP adapters (WAN Miniport) are almost always VPN on laptops
		vpnName := matchVPNPattern(combined)
		if vpnName == "" {
			vpnName = "Windows VPN"
		}
		return VPNClassification{
			IsVPN:     true,
			VPNName:   vpnName,
			VPNType:   "ppp",
			Interface: iface.name,
		}

	case ifTypePropVirtual:
		// Proprietary virtual adapters: VPN only if name matches a known pattern
		vpnName := matchVPNPattern(combined)
		if vpnName == "" {
			return VPNClassification{}
		}
		return VPNClassification{
			IsVPN:     true,
			VPNName:   vpnName,
			VPNType:   "prop_virtual",
			Interface: iface.name,
		}

	case ifTypeTunnel:
		// Tunnel adapters: VPN candidate, verify by name
		vpnName := matchVPNPattern(combined)
		if vpnName == "" {
			return VPNClassification{}
		}
		return VPNClassification{
			IsVPN:     true,
			VPNName:   vpnName,
			VPNType:   "tunnel",
			Interface: iface.name,
		}

	case ifTypeEthernetCSMACD:
		// Ethernet adapters: VPN only if name contains tap/tun/vpn
		vpnName := matchVPNPattern(combined)
		if vpnName == "" {
			// Also check for generic tap/tun indicators
			if strings.Contains(combined, "tap") || strings.Contains(combined, "tun") || strings.Contains(combined, "vpn") {
				return VPNClassification{
					IsVPN:     true,
					VPNName:   "Unknown VPN",
					VPNType:   "ethernet_tap",
					Interface: iface.name,
				}
			}
			return VPNClassification{}
		}
		return VPNClassification{
			IsVPN:     true,
			VPNName:   vpnName,
			VPNType:   "ethernet_tap",
			Interface: iface.name,
		}
	}

	return VPNClassification{}
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
