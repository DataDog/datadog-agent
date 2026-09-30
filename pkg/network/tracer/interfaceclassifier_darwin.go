// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"net"
	"strings"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// InterfaceClassification holds interface metadata looked up by interface index.
type InterfaceClassification struct {
	InterfaceName string // BSD name, e.g. "en0", "utun3", "lo0"
	InterfaceType string // name-prefix class, e.g. "ethernet_csmacd", "tunnel"
}

// cachedInterface stores the name and class for one Darwin interface index.
type cachedInterface struct {
	name      string
	ifaceType string
}

// InterfaceClassifier resolves interface indices to interface metadata by
// periodically refreshing interface information from the OS.
type InterfaceClassifier struct {
	mu      sync.RWMutex
	ifCache map[uint32]cachedInterface // ifIndex -> metadata
	done    chan struct{}
	stop    sync.Once
}

// NewInterfaceClassifier creates a classifier and starts a background refresh loop.
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

// refreshCache queries the OS interface list and rebuilds the cache.
// A failed query leaves the previous cache in place.
func (c *InterfaceClassifier) refreshCache() {
	ifaces, err := net.Interfaces()
	if err != nil {
		log.Warnf("interface_classifier: failed to list interfaces: %v", err)
		return
	}

	newCache := make(map[uint32]cachedInterface, len(ifaces))
	for _, iface := range ifaces {
		if iface.Index <= 0 || iface.Name == "" {
			continue
		}
		newCache[uint32(iface.Index)] = cachedInterface{
			name:      iface.Name,
			ifaceType: interfaceTypeFor(iface.Name, iface.Flags),
		}
	}

	c.mu.Lock()
	old := c.ifCache
	c.ifCache = newCache
	c.mu.Unlock()

	for idx, ci := range newCache {
		if _, existed := old[idx]; !existed {
			log.Debugf("interface_classifier: cached new interface idx=%d type=%s name=%q",
				idx, ci.ifaceType, ci.name)
		}
	}
}

// Classify returns interface metadata for the given interface index. Index 0
// and an index missing from the cache return an empty classification.
func (c *InterfaceClassifier) Classify(interfaceIndex uint32) InterfaceClassification {
	if c == nil || interfaceIndex == 0 {
		return InterfaceClassification{}
	}
	c.mu.RLock()
	iface, ok := c.ifCache[interfaceIndex]
	c.mu.RUnlock()
	if !ok || iface.name == "" {
		return InterfaceClassification{}
	}
	return InterfaceClassification{
		InterfaceName: iface.name,
		InterfaceType: iface.ifaceType,
	}
}

// Close stops the background refresh goroutine. It is safe to call more than once.
func (c *InterfaceClassifier) Close() {
	if c == nil {
		return
	}
	c.stop.Do(func() {
		close(c.done)
	})
}

// interfaceTypeFor classifies a BSD interface from flags and name prefix.
// utun is often IFT_OTHER, so the name is used instead of the raw ifi_type.
// en* is not split into Wi-Fi and Ethernet.
func interfaceTypeFor(name string, flags net.Flags) string {
	if flags&net.FlagLoopback != 0 || strings.HasPrefix(name, "lo") {
		return "software_loopback"
	}
	for _, prefix := range []string{"utun", "gif", "stf", "ipsec", "ppp"} {
		if strings.HasPrefix(name, prefix) {
			return "tunnel"
		}
	}
	switch {
	case strings.HasPrefix(name, "bridge"):
		return "bridge"
	case strings.HasPrefix(name, "vlan"):
		return "vlan"
	case strings.HasPrefix(name, "en"):
		return "ethernet_csmacd"
	default:
		return "other"
	}
}
