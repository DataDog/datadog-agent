// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build darwin

package tracer

import (
	"net"
	"net/netip"
	"sort"
	"strings"
	"sync"
	"time"

	"go4.org/intern"
	"golang.org/x/net/route"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// appendInterfaceTags copies c.Tags and adds the interface tags when absent.
// The copy keeps the append off a tag slice shared with the connection backend.
func appendInterfaceTags(c *network.ConnectionStats, name, typ string) {
	if c == nil || name == "" || typ == "" {
		return
	}
	tags := make([]*intern.Value, len(c.Tags), len(c.Tags)+2)
	copy(tags, c.Tags)
	tags = appendNewInterfaceTag(tags, intern.GetByString("interface_name:"+name))
	tags = appendNewInterfaceTag(tags, intern.GetByString("interface_type:"+typ))
	c.Tags = tags
}

// appendNewInterfaceTag appends tag when tags does not already contain that interned value.
func appendNewInterfaceTag(tags []*intern.Value, tag *intern.Value) []*intern.Value {
	for _, existing := range tags {
		if existing == tag {
			return tags
		}
	}
	return append(tags, tag)
}

const interfaceTagRefreshInterval = 30 * time.Second

// darwinInterface is one operating-system interface. name is the BSD name.
type darwinInterface struct {
	name  string
	typ   string
	index int
}

// darwinRoute is a cached prefix that names an interface by index.
type darwinRoute struct {
	prefix  netip.Prefix
	index   int
	ifScope bool
}

// interfaceTagger resolves a connection's local address, or the route to its
// peer, to an interface name and type.
type interfaceTagger struct {
	mu       sync.RWMutex
	byAddr   map[netip.Addr]darwinInterface
	byName   map[string]darwinInterface
	byIndex  map[int]darwinInterface
	routes   []darwinRoute
	done     chan struct{}
	stopOnce sync.Once
}

// newInterfaceTagger refreshes once, then every 30 seconds.
func newInterfaceTagger() *interfaceTagger {
	t := &interfaceTagger{done: make(chan struct{})}
	t.refresh()
	go func() {
		ticker := time.NewTicker(interfaceTagRefreshInterval)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				t.refresh()
			case <-t.done:
				return
			}
		}
	}()
	return t
}

// Close stops the refresh goroutine. It is safe to call more than once.
func (t *interfaceTagger) Close() {
	if t == nil || t.done == nil {
		return
	}
	t.stopOnce.Do(func() { close(t.done) })
}

// classify returns the interface for local, or the route toward remote when
// local is unspecified. A specific local address that the cache does not own
// is left unresolved.
func (t *interfaceTagger) classify(local, remote netip.Addr) (name, typ string, index int) {
	if t == nil {
		return "", "", 0
	}
	zoned := local
	local = local.Unmap()
	remote = remote.Unmap()

	t.mu.RLock()
	defer t.mu.RUnlock()

	if zone := zoned.Zone(); zone != "" {
		if iface, ok := t.byName[zone]; ok {
			return iface.name, iface.typ, iface.index
		}
	}
	if local.IsValid() && !local.IsUnspecified() {
		iface, ok := t.byAddr[local.WithZone("")]
		if !ok {
			return "", "", 0
		}
		return iface.name, iface.typ, iface.index
	}
	if !remote.IsValid() || remote.IsUnspecified() {
		return "", "", 0
	}
	return t.matchRoute(remote)
}

// matchRoute returns the interface for the longest route that contains remote.
// An unscoped route wins only when it has that same length. Scoped routes of
// that length which name different interfaces return no tag.
func (t *interfaceTagger) matchRoute(remote netip.Addr) (string, string, int) {
	var (
		scoped      bool
		scopedIndex int
		scopedBits  int
	)
	for _, r := range t.routes {
		if !r.prefix.Contains(remote) {
			continue
		}
		iface, ok := t.byIndex[r.index]
		if !ok {
			continue
		}
		// Routes are longest-prefix first. A shorter unscoped route must not
		// replace a longer scoped match. An unscoped route wins only when
		// it has the same length as that match.
		if scoped && r.prefix.Bits() != scopedBits {
			break
		}
		if !r.ifScope {
			return iface.name, iface.typ, iface.index
		}
		if !scoped {
			scoped = true
			scopedIndex = iface.index
			scopedBits = r.prefix.Bits()
			continue
		}
		if iface.index != scopedIndex {
			return "", "", 0
		}
	}
	if !scoped {
		return "", "", 0
	}
	iface := t.byIndex[scopedIndex]
	return iface.name, iface.typ, iface.index
}

// refresh reloads interface addresses. A failed listing keeps the previous cache.
// A failed route dump keeps the previous routes and still publishes the new addresses.
func (t *interfaceTagger) refresh() {
	byAddr, byName, byIndex, err := loadInterfaceAddresses()
	if err != nil {
		log.Warnf("interface_classifier: failed to refresh interfaces: %v", err)
		return
	}
	routes, err := loadRoutes(byIndex)
	if err != nil {
		log.Warnf("interface_classifier: failed to refresh routes: %v", err)
	}

	t.mu.Lock()
	old := t.byName
	t.byAddr = byAddr
	t.byName = byName
	t.byIndex = byIndex
	if err == nil {
		t.routes = routes
	}
	t.mu.Unlock()

	for name, iface := range byName {
		if _, existed := old[name]; existed {
			continue
		}
		log.Debugf("interface_classifier: cached new interface idx=%d type=%s name=%q",
			iface.index, iface.typ, iface.name)
	}
}

// loadInterfaceAddresses lists host interfaces and their addresses. An address
// present on more than one interface is omitted.
func loadInterfaceAddresses() (map[netip.Addr]darwinInterface, map[string]darwinInterface, map[int]darwinInterface, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, nil, nil, err
	}
	byAddr := make(map[netip.Addr]darwinInterface)
	ambiguous := make(map[netip.Addr]struct{})
	byName := make(map[string]darwinInterface, len(ifaces))
	byIndex := make(map[int]darwinInterface, len(ifaces))
	for _, iface := range ifaces {
		if iface.Index <= 0 || iface.Name == "" {
			continue
		}
		info := darwinInterface{
			name:  iface.Name,
			typ:   interfaceTypeFor(iface.Name),
			index: iface.Index,
		}
		byName[iface.Name] = info
		byIndex[iface.Index] = info
		addrs, err := iface.Addrs()
		if err != nil {
			log.Debugf("interface_classifier: addresses for %s: %v", iface.Name, err)
			continue
		}
		for _, addr := range addrs {
			ip, ok := ipFromNetAddr(addr)
			if !ok || !ip.IsValid() || ip.IsUnspecified() {
				continue
			}
			addInterfaceAddress(byAddr, ambiguous, ip.Unmap().WithZone(""), info)
		}
	}
	return byAddr, byName, byIndex, nil
}

// addInterfaceAddress records addr for info. A second interface with the same
// address removes the first record.
func addInterfaceAddress(byAddr map[netip.Addr]darwinInterface, ambiguous map[netip.Addr]struct{}, addr netip.Addr, info darwinInterface) {
	if _, dup := ambiguous[addr]; dup {
		return
	}
	if _, exists := byAddr[addr]; exists {
		delete(byAddr, addr)
		ambiguous[addr] = struct{}{}
		return
	}
	byAddr[addr] = info
}

// ipFromNetAddr returns the unmapped IP from a net.IPNet or net.IPAddr.
func ipFromNetAddr(addr net.Addr) (netip.Addr, bool) {
	var ip net.IP
	switch v := addr.(type) {
	case *net.IPNet:
		ip = v.IP
	case *net.IPAddr:
		ip = v.IP
	default:
		return netip.Addr{}, false
	}
	parsed, ok := netip.AddrFromSlice(ip)
	if !ok {
		return netip.Addr{}, false
	}
	return parsed.Unmap(), true
}

// loadRoutes reads the host routing table and keeps routes for known interfaces.
func loadRoutes(byIndex map[int]darwinInterface) ([]darwinRoute, error) {
	buf, err := route.FetchRIB(unix.AF_UNSPEC, route.RIBType(unix.NET_RT_DUMP2), 0)
	if err != nil {
		return nil, err
	}
	msgs, err := route.ParseRIB(route.RIBType(unix.NET_RT_DUMP2), buf)
	if err != nil {
		return nil, err
	}
	return routesFromMessages(msgs, byIndex), nil
}

// routesFromMessages keeps routes that name a known interface. Reject and
// blackhole routes are dropped.
func routesFromMessages(msgs []route.Message, byIndex map[int]darwinInterface) []darwinRoute {
	routes := make([]darwinRoute, 0, len(msgs))
	for _, msg := range msgs {
		rm, ok := msg.(*route.RouteMessage)
		if !ok || rm == nil {
			continue
		}
		if rm.Flags&(unix.RTF_REJECT|unix.RTF_BLACKHOLE) != 0 || rm.Index <= 0 {
			continue
		}
		if _, ok := byIndex[rm.Index]; !ok {
			continue
		}
		prefix, ok := routePrefix(rm)
		if !ok {
			continue
		}
		routes = append(routes, darwinRoute{
			prefix:  prefix,
			index:   rm.Index,
			ifScope: rm.Flags&unix.RTF_IFSCOPE != 0,
		})
	}
	orderRoutes(routes)
	return routes
}

// orderRoutes sorts routes longest prefix first. At the same length, an
// unscoped route comes before a scoped route.
func orderRoutes(routes []darwinRoute) {
	sort.SliceStable(routes, func(i, j int) bool {
		if routes[i].prefix.Bits() != routes[j].prefix.Bits() {
			return routes[i].prefix.Bits() > routes[j].prefix.Bits()
		}
		return !routes[i].ifScope && routes[j].ifScope
	})
}

// routePrefix returns the destination prefix of a route message.
func routePrefix(rm *route.RouteMessage) (netip.Prefix, bool) {
	dst, ok := ipFromRouteAddr(addrAt(rm.Addrs, unix.RTAX_DST))
	if !ok || !dst.IsValid() {
		return netip.Prefix{}, false
	}
	dst = dst.Unmap().WithZone("")
	mask, hasMask := ipFromRouteAddr(addrAt(rm.Addrs, unix.RTAX_NETMASK))
	if hasMask {
		mask = mask.Unmap().WithZone("")
		if mask.BitLen() != dst.BitLen() {
			hasMask = false
		}
	}
	var maskAddr netip.Addr
	if hasMask {
		maskAddr = mask
	}
	bits := prefixLen(dst, maskAddr, rm.Flags&unix.RTF_HOST != 0)
	prefix, err := dst.Prefix(bits)
	if err != nil {
		return netip.Prefix{}, false
	}
	return prefix, true
}

// addrAt returns the address at index i, or nil when i is outside addrs.
func addrAt(addrs []route.Addr, i int) route.Addr {
	if i < 0 || i >= len(addrs) {
		return nil
	}
	return addrs[i]
}

// ipFromRouteAddr returns the IP from an IPv4 or IPv6 route address.
func ipFromRouteAddr(addr route.Addr) (netip.Addr, bool) {
	switch v := addr.(type) {
	case *route.Inet4Addr:
		return netip.AddrFrom4(v.IP), true
	case *route.Inet6Addr:
		return netip.AddrFrom16(v.IP), true
	default:
		return netip.Addr{}, false
	}
}

// prefixLen returns the prefix length of dst. A host route uses the full address length.
func prefixLen(dst, mask netip.Addr, host bool) int {
	if host {
		return dst.BitLen()
	}
	if !mask.IsValid() {
		if dst.IsUnspecified() {
			return 0
		}
		return dst.BitLen()
	}
	bits := 0
	for _, b := range mask.AsSlice() {
		if b == 0xff {
			bits += 8
			continue
		}
		for b&0x80 != 0 {
			bits++
			b <<= 1
		}
		break
	}
	if bits > dst.BitLen() {
		return dst.BitLen()
	}
	return bits
}

// interfaceTypeFor maps a BSD interface name onto the Windows interface_type
// strings. utun is classified from its name because the kernel type is often
// IFT_OTHER. en* covers both Ethernet and Wi-Fi.
func interfaceTypeFor(name string) string {
	switch {
	case strings.HasPrefix(name, "lo"):
		return "software_loopback"
	case strings.HasPrefix(name, "ppp"):
		return "ppp"
	case strings.HasPrefix(name, "utun"), strings.HasPrefix(name, "gif"), strings.HasPrefix(name, "stf"),
		strings.HasPrefix(name, "ipsec"):
		return "tunnel"
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
