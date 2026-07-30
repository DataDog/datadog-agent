// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package npcollectorimpl

import (
	"fmt"
	"net"
	"net/netip"
	"sync"
	"time"

	cache "github.com/patrickmn/go-cache"
)

const (
	localIPCacheTTL         = time.Minute
	localIPCacheMaxStaleAge = 10 * time.Minute

	localIPCacheSnapshotKey       = "local_interface_ips"
	localIPCacheRefreshAttemptKey = "local_interface_ips_refresh_attempt"
)

type localIPDiscoveryFunc func() (map[netip.Addr]struct{}, error)

type localIPCache struct {
	mu          sync.Mutex
	discover    localIPDiscoveryFunc
	ttl         time.Duration
	maxStaleAge time.Duration
	items       *cache.Cache
}

func newLocalIPCache(discover localIPDiscoveryFunc) *localIPCache {
	return &localIPCache{
		discover:    discover,
		ttl:         localIPCacheTTL,
		maxStaleAge: localIPCacheMaxStaleAge,
		items:       cache.New(cache.NoExpiration, localIPCacheTTL),
	}
}

func (c *localIPCache) contains(addr netip.Addr) (bool, error) {
	if c == nil || !addr.IsValid() {
		return false, nil
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if _, recentlyAttempted := c.items.Get(localIPCacheRefreshAttemptKey); !recentlyAttempted {
		c.items.Set(localIPCacheRefreshAttemptKey, struct{}{}, c.ttl)
		ips, err := c.discover()
		if err != nil {
			return c.containsCached(addr), err
		}
		c.items.Set(localIPCacheSnapshotKey, ips, c.maxStaleAge)
		return containsLocalIP(ips, addr), nil
	}

	return c.containsCached(addr), nil
}

func (c *localIPCache) containsCached(addr netip.Addr) bool {
	cachedIPs, found := c.items.Get(localIPCacheSnapshotKey)
	if !found {
		return false
	}
	ips, ok := cachedIPs.(map[netip.Addr]struct{})
	if !ok {
		return false
	}
	return containsLocalIP(ips, addr)
}

func containsLocalIP(ips map[netip.Addr]struct{}, addr netip.Addr) bool {
	_, ok := ips[addr.Unmap()]
	return ok
}

func discoverLocalInterfaceIPs() (map[netip.Addr]struct{}, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}

	ips := make(map[netip.Addr]struct{})
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, fmt.Errorf("get addresses for interface %s: %w", iface.Name, err)
		}

		for _, addr := range addrs {
			ip, ok := interfaceAddrIP(addr)
			if !ok {
				continue
			}
			netipAddr, ok := netipAddrFromIP(ip)
			if !ok {
				continue
			}
			ips[netipAddr.Unmap()] = struct{}{}
		}
	}

	return ips, nil
}

func interfaceAddrIP(addr net.Addr) (net.IP, bool) {
	switch typedAddr := addr.(type) {
	case *net.IPNet:
		return typedAddr.IP, true
	case *net.IPAddr:
		return typedAddr.IP, true
	default:
		return nil, false
	}
}

func netipAddrFromIP(ip net.IP) (netip.Addr, bool) {
	if ip == nil {
		return netip.Addr{}, false
	}
	if v4 := ip.To4(); v4 != nil {
		return netip.AddrFromSlice(v4)
	}
	if v6 := ip.To16(); v6 != nil {
		return netip.AddrFromSlice(v6)
	}
	return netip.Addr{}, false
}
