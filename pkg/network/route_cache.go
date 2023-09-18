// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"fmt"
	"net"
	"sync"
	"syscall"
	"time"

	"github.com/golang/groupcache/lru"
	"github.com/vishvananda/netlink"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network/config"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

type routeKey struct {
	source, dest util.Address
	netns        uint32
	connFamily   ConnectionFamily
}

// Route stores info for a route table entry
type Route struct {
	Gateway util.Address
	IfIndex int
}

type routeTTL struct {
	eta   int64
	entry Route
}

type routeCache struct {
	sync.Mutex
	cache  *lru.Cache
	router Router
	ttl    time.Duration
}

const (
	defaultTTL                    = 2 * time.Minute
	routeCacheTelemetryModuleName = "network_tracer__gateway_lookup_route_cache"
	routerTelemetryModuleName     = "network_tracer__gateway_lookup_route_cache_router"
)

var routeCacheTelemetry = struct {
	size    telemetry.Gauge
	misses  telemetry.Counter
	lookups telemetry.Counter
	expires telemetry.Counter
	evicts  telemetry.Counter

	netlinkLookups telemetry.Counter
	netlinkErrors  telemetry.Counter
	netlinkMisses  telemetry.Counter

	ifCacheLookups telemetry.Counter
	ifCacheMisses  telemetry.Counter
	ifCacheSize    telemetry.Counter
	ifCacheErrors  telemetry.Counter
}{
	telemetry.NewGauge(routeCacheTelemetryModuleName, "size", []string{}, "Gauge measuring the size of the route cache"),
	telemetry.NewCounter(routeCacheTelemetryModuleName, "misses", []string{}, "Counter measuring the number of route cache misses"),
	telemetry.NewCounter(routeCacheTelemetryModuleName, "lookups", []string{}, "Counter measuring the number of route cache lookups"),
	telemetry.NewCounter(routeCacheTelemetryModuleName, "expires", []string{}, "Counter measuring the number of route cache expirations"),
	telemetry.NewCounter(routeCacheTelemetryModuleName, "evicts", []string{}, "Counter measuring the number of route cache evicts"),

	telemetry.NewCounter(routerTelemetryModuleName, "netlink_lookups", []string{}, "Counter measuring the number of netlink lookups"),
	telemetry.NewCounter(routerTelemetryModuleName, "netlink_errors", []string{"error"}, "Counter measuring the number of netlink errors"),
	telemetry.NewCounter(routerTelemetryModuleName, "netlink_misses", []string{}, "Counter measuring the number of netlink misses"),

	telemetry.NewCounter(routerTelemetryModuleName, "if_cache_lookups", []string{}, "Counter measuring the number of interface cache lookups"),
	telemetry.NewCounter(routerTelemetryModuleName, "if_cache_misses", []string{}, "Counter measuring the number of interface cache misses"),
	telemetry.NewCounter(routerTelemetryModuleName, "if_cache_size", []string{}, "Counter measuring the size of the interface cache"),
	telemetry.NewCounter(routerTelemetryModuleName, "if_cache_errors", []string{"error"}, "Counter measuring the number of interface cache errors"),
}

// RouteCache is the interface to a cache that stores routes for a given (source, destination, net ns) tuple
type RouteCache interface {
	Get(source, dest util.Address, netns uint32) (Route, bool)
	Close()
}

// Router is an interface to get a route for a (source, destination, net ns) tuple
type Router interface {
	Route(source, dest util.Address, netns uint32) (Route, bool)
	Close()
}

// NewRouteCache creates a new RouteCache
func NewRouteCache(size int, router Router) RouteCache {
	return newRouteCache(size, router, defaultTTL)
}

// newRouteCache is a private method used primarily for testing
func newRouteCache(size int, router Router, ttl time.Duration) *routeCache {
	if router == nil {
		return nil
	}

	rc := &routeCache{
		cache:  lru.New(size),
		router: router,
		ttl:    ttl,
	}

	rc.cache.OnEvicted = func(_ lru.Key, _ interface{}) {
		routeCacheTelemetry.evicts.Inc()
	}

	return rc
}

func (c *routeCache) Close() {
	c.Lock()
	defer c.Unlock()

	c.cache.Clear()
	c.router.Close()
}

func (c *routeCache) Get(source, dest util.Address, netns uint32) (Route, bool) {
	c.Lock()
	defer func() {
		routeCacheTelemetry.size.Set(float64(c.cache.Len()))
		c.Unlock()
	}()

	routeCacheTelemetry.lookups.Inc()
	k := newRouteKey(source, dest, netns)
	if entry, ok := c.cache.Get(k); ok {
		if time.Now().Unix() < entry.(*routeTTL).eta {
			return entry.(*routeTTL).entry, ok
		}

		routeCacheTelemetry.expires.Inc()
		c.cache.Remove(k)
	} else {
		routeCacheTelemetry.misses.Inc()
	}

	if r, ok := c.router.Route(source, dest, netns); ok {
		entry := &routeTTL{
			eta:   time.Now().Add(c.ttl).Unix(),
			entry: r,
		}

		c.cache.Add(k, entry)
		return r, true
	}

	return Route{}, false
}

func newRouteKey(source, dest util.Address, netns uint32) routeKey {
	k := routeKey{netns: netns, source: source, dest: dest}

	switch dest.Len() {
	case 4:
		k.connFamily = AFINET
	case 16:
		k.connFamily = AFINET6
	}
	return k
}

type ifkey struct {
	ip    util.Address
	netns uint32
}

type ifEntry struct {
	index    int
	loopback bool
}

type netlinkRouter struct {
	rootNs   uint32
	ioctlFD  int
	ifcache  *lru.Cache
	nlHandle *netlink.Handle
}

// NewNetlinkRouter create a Router that queries routes via netlink
func NewNetlinkRouter(cfg *config.Config) (Router, error) {
	rootNs, err := cfg.GetRootNetNs()
	if err != nil {
		return nil, err
	}
	defer rootNs.Close()

	rootNsIno, err := kernel.GetInoForNs(rootNs)
	if err != nil {
		return nil, fmt.Errorf("netlink gw cache backing: could not get root net ns: %w", err)
	}

	var fd int
	var nlHandle *netlink.Handle
	err = kernel.WithNS(rootNs, func() (sockErr error) {
		if fd, err = unix.Socket(unix.AF_INET, unix.SOCK_STREAM, 0); err != nil {
			return err
		}

		nlHandle, err = netlink.NewHandle(unix.NETLINK_ROUTE)
		return err
	})

	if err != nil {
		return nil, err
	}

	nr := &netlinkRouter{
		rootNs:  rootNsIno,
		ioctlFD: fd,
		// ifcache should ideally fit all interfaces on a given node
		ifcache:  lru.New(128),
		nlHandle: nlHandle,
	}

	return nr, nil
}

func (n *netlinkRouter) Close() {
	n.ifcache.Clear()
	unix.Close(n.ioctlFD)
	n.nlHandle.Close()
}

func (n *netlinkRouter) Route(source, dest util.Address, netns uint32) (Route, bool) {
	var iifIndex int

	srcBuf := util.IPBufferPool.Get().(*[]byte)
	dstBuf := util.IPBufferPool.Get().(*[]byte)
	defer func() {
		util.IPBufferPool.Put(srcBuf)
		util.IPBufferPool.Put(dstBuf)
	}()

	srcIP := util.NetIPFromAddress(source, *srcBuf)
	if n.rootNs != netns {
		// if its a non-root ns, we're dealing with traffic from
		// a container most likely, and so need to find out
		// which interface is associated with the ns

		// get input interface for src ip
		iif := n.getInterface(source, srcIP, netns)
		if iif == nil || iif.index == 0 {
			return Route{}, false
		}

		if !iif.loopback {
			iifIndex = iif.index
		}
	}

	routeCacheTelemetry.netlinkLookups.Inc()
	dstIP := util.NetIPFromAddress(dest, *dstBuf)
	routes, err := n.nlHandle.RouteGetWithOptions(
		dstIP,
		&netlink.RouteGetOptions{
			SrcAddr:  srcIP,
			IifIndex: iifIndex,
		})

	if err != nil {
		errno, ok := counterIncWithTag(routeCacheTelemetry.netlinkErrors, err)
		if iifIndex > 0 {
			if ok && (errno == syscall.EINVAL || errno == syscall.ENODEV) {
				// invalidate interface cache entry as this may have been the cause of the netlink error
				n.removeInterface(source, netns)
			}
		}
		log.Debugf("Error getting route via netlink with sourceIP %s, dest IP %s and interface index %d : %s", srcIP, dstIP, iifIndex, err)
	} else if len(routes) != 1 {
		log.Debugf("Did not get exactly one route with sourceIP %s, dest IP %s and interface index %d, got %d routes", srcIP, dstIP, iifIndex, len(routes))
		routeCacheTelemetry.netlinkMisses.Inc()
	}
	if err != nil || len(routes) != 1 {
		log.Tracef("could not get route for src=%s dest=%s err=%s routes=%+v", source, dest, err, routes)
		return Route{}, false
	}

	r := routes[0]
	log.Tracef("route for src=%s dst=%s: scope=%s gw=%+v if=%d", source, dest, r.Scope, r.Gw, r.LinkIndex)
	return Route{
		Gateway: util.AddressFromNetIP(r.Gw),
		IfIndex: r.LinkIndex,
	}, true
}

func (n *netlinkRouter) removeInterface(srcAddress util.Address, netns uint32) {
	key := ifkey{ip: srcAddress, netns: netns}
	n.ifcache.Remove(key)
}

func (n *netlinkRouter) getInterface(srcAddress util.Address, srcIP net.IP, netns uint32) *ifEntry {
	routeCacheTelemetry.ifCacheLookups.Inc()

	key := ifkey{ip: srcAddress, netns: netns}
	if entry, ok := n.ifcache.Get(key); ok {
		return entry.(*ifEntry)
	}
	routeCacheTelemetry.ifCacheMisses.Inc()

	routeCacheTelemetry.netlinkLookups.Inc()
	routes, err := n.nlHandle.RouteGet(srcIP)
	if err != nil {
		_, _ = counterIncWithTag(routeCacheTelemetry.netlinkErrors, err)
		log.Debugf("Error getting route via netlink %s: %s", srcIP, err)
		return nil
	} else if len(routes) != 1 {
		log.Debugf("Did not get exactly one route for %s, got %d routes", srcIP, len(routes))
		routeCacheTelemetry.netlinkMisses.Inc()
		return nil
	}

	ifr, err := unix.NewIfreq("")
	if err != nil {
		_, _ = counterIncWithTag(routeCacheTelemetry.ifCacheErrors, err)
		return nil
	}

	ifr.SetUint32(uint32(routes[0].LinkIndex))
	// first get the name of the interface. this is
	// necessary to make the subsequent request to
	// get the link flags
	if err = unix.IoctlIfreq(n.ioctlFD, unix.SIOCGIFNAME, ifr); err != nil {
		_, _ = counterIncWithTag(routeCacheTelemetry.ifCacheErrors, err)
		log.Debugf("error getting interface name for link index %d, src ip %s: %s", routes[0].LinkIndex, srcIP, err)
		return nil
	}
	if err = unix.IoctlIfreq(n.ioctlFD, unix.SIOCGIFFLAGS, ifr); err != nil {
		_, _ = counterIncWithTag(routeCacheTelemetry.ifCacheErrors, err)
		log.Debugf("error getting interface flags for link index %d, src ip %s: %s", routes[0].LinkIndex, srcIP, err)
		return nil
	}

	iff := &ifEntry{index: routes[0].LinkIndex, loopback: (ifr.Uint16() & unix.IFF_LOOPBACK) != 0}
	log.Tracef("adding interface entry, key=%+v, entry=%v", key, *iff)
	n.ifcache.Add(key, iff)
	routeCacheTelemetry.ifCacheSize.Inc()
	return iff
}

func counterIncWithTag(counter telemetry.Counter, err error) (errno syscall.Errno, ok bool) {
	if errno, ok = err.(syscall.Errno); ok {
		if tag := unix.ErrnoName(errno); tag != "" {
			counter.Inc(tag)
			return
		}
	}
	counter.Inc()
	return
}
