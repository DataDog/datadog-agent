// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build windows && npm

package tracer

import (
	"sync"
	"time"
	"unicode/utf16"

	"golang.org/x/sys/windows"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/util/winutil/iphelper"
)

// InterfaceClassification holds interface metadata looked up by interface index.
// InterfaceType is the raw IANA ifType value
// (https://www.iana.org/assignments/ianaiftype-mib/ianaiftype-mib); mapping to a
// human-readable name (and any VPN identification) is performed downstream.
type InterfaceClassification struct {
	InterfaceName string
	InterfaceType uint32
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
			log.Debugf("interface_classifier: cached new interface idx=%d type=%d name=%q descr=%q",
				idx, ci.ifType, ci.name, ci.descr)
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
		InterfaceType: iface.ifType,
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
