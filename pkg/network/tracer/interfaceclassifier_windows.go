// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"fmt"
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
	ifTypePPP            = 23  // IF_TYPE_PPP
	ifTypeSoftwareLoop   = 24  // IF_TYPE_SOFTWARE_LOOPBACK
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

// InterfaceClassification holds interface metadata looked up by interface index.
// VPN identification is performed downstream (in the backend) from these tags.
type InterfaceClassification struct {
	InterfaceName string // friendly name, e.g. "Intel Wi-Fi 6 AX201", "WireGuard Tunnel"
	InterfaceType string // human-readable ifType, e.g. "ethernet", "wifi", "prop_virtual"
}

// cachedInterface stores minimal info about a network interface
type cachedInterface struct {
	ifType uint32
	name   string // from MibIfRow.Name (UTF-16 friendly name)
	descr  string // from MibIfRow.Descr
}

// friendlyName returns a human-readable interface name, preferring the
// description (e.g. "WireGuard Tunnel") over the raw device path.
func (ci cachedInterface) friendlyName() string {
	if ci.descr != "" {
		return ci.descr
	}
	return ci.name
}

// InterfaceClassifier resolves interface indices to interface metadata by
// periodically refreshing interface information from the OS.
type InterfaceClassifier struct {
	mu      sync.RWMutex
	ifCache map[uint32]cachedInterface // ifIndex -> metadata
	done    chan struct{}
}

// NewInterfaceClassifier creates a new classifier and starts a background refresh loop
func NewInterfaceClassifier() *InterfaceClassifier {
	c := &InterfaceClassifier{
		ifCache: make(map[uint32]cachedInterface),
		done:    make(chan struct{}),
	}
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

// refreshCache queries the OS interface table and rebuilds the cache.
// Logs a debug message whenever an interface appears that wasn't in the
// previous cache.
func (c *InterfaceClassifier) refreshCache() {
	table, err := iphelper.GetIFTable()
	if err != nil {
		log.Warnf("interface_classifier: failed to get interface table: %v", err)
		return
	}

	newCache := make(map[uint32]cachedInterface, len(table))
	for idx, row := range table {
		newCache[idx] = cachedInterface{
			ifType: row.Type,
			name:   mibIfRowName(row),
			descr:  mibIfRowDescr(row),
		}
	}

	c.mu.Lock()
	old := c.ifCache
	c.ifCache = newCache
	c.mu.Unlock()

	for idx, ci := range newCache {
		if _, existed := old[idx]; !existed {
			log.Debugf("interface_classifier: cached new interface idx=%d type=%s name=%q descr=%q",
				idx, ifTypeName(ci.ifType), ci.name, ci.descr)
		}
	}
}

// Classify returns interface metadata for the given interface index. If the
// index is not in the cache, the returned InterfaceClassification is empty.
func (c *InterfaceClassifier) Classify(interfaceIndex uint32) InterfaceClassification {
	c.mu.RLock()
	iface, ok := c.ifCache[interfaceIndex]
	c.mu.RUnlock()

	if !ok {
		return InterfaceClassification{}
	}
	return InterfaceClassification{
		InterfaceName: iface.friendlyName(),
		InterfaceType: ifTypeName(iface.ifType),
	}
}

// Close stops the background refresh goroutine
func (c *InterfaceClassifier) Close() {
	close(c.done)
}

// mibIfRowName extracts the UTF-16 Name field from a MibIfRow as a Go string
func mibIfRowName(row windows.MibIfRow) string {
	// Name is [256]uint16, find the null terminator
	nameSlice := row.Name[:]
	end := len(nameSlice)
	for i, c := range nameSlice {
		if c == 0 {
			end = i
			break
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
