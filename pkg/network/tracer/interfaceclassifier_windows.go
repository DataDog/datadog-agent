// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"bytes"
	"strconv"
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iphelper"
)

// ifTypeToString maps the ifType values that Windows commonly reports via
// MIB_IFROW.dwType to snake_case names derived from the IF_TYPE_* macros.
// References:
//   - https://learn.microsoft.com/en-us/windows/win32/api/ifmib/ns-ifmib-mib_ifrow
//     (Microsoft documentation listing the values typically reported on Windows)
//   - https://github.com/tpn/winsdk-10/blob/master/Include/10.0.14393.0/shared/ipifcons.h
//     (Windows SDK ipifcons.h, the canonical source of the IF_TYPE_* macros)
//
// Values outside this list fall through to ifTypeName as the raw integer.
var ifTypeToString = map[uint32]string{
	1:   "other",               // IF_TYPE_OTHER
	6:   "ethernet_csmacd",     // IF_TYPE_ETHERNET_CSMACD
	9:   "iso88025_token_ring", // IF_TYPE_ISO88025_TOKENRING
	23:  "ppp",                 // IF_TYPE_PPP
	24:  "software_loopback",   // IF_TYPE_SOFTWARE_LOOPBACK
	37:  "atm",                 // IF_TYPE_ATM
	53:  "prop_virtual",        // IF_TYPE_PROP_VIRTUAL
	71:  "ieee80211",           // IF_TYPE_IEEE80211
	131: "tunnel",              // IF_TYPE_TUNNEL
	144: "ieee1394",            // IF_TYPE_IEEE1394
	237: "ieee80216_wman",      // IF_TYPE_IEEE80216_WMAN (WiMAX mobile broadband)
	243: "wwan_pp",             // IF_TYPE_WWANPP (GSM-based mobile broadband)
	244: "wwan_pp2",            // IF_TYPE_WWANPP2 (CDMA-based mobile broadband)
}

// ifTypeName returns a snake_case string for a known Windows ifType value.
// Values outside the documented set are returned as their raw integer so
// new types can be identified downstream without an agent release.
func ifTypeName(ifType uint32) string {
	if name, ok := ifTypeToString[ifType]; ok {
		return name
	}
	return strconv.FormatUint(uint64(ifType), 10)
}

// InterfaceClassification holds interface metadata looked up by interface index.
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

// mibIfRowDescr extracts the Descr field from a MibIfRow as a Go string.
// dwDescrLen includes the null terminator, so we scan for the NUL byte instead
// of trusting the length to avoid a trailing \x00 in the returned string.
func mibIfRowDescr(row windows.MibIfRow) string {
	descr := row.Descr[:]
	if i := bytes.IndexByte(descr, 0); i >= 0 {
		return string(descr[:i])
	}
	return string(descr)
}
