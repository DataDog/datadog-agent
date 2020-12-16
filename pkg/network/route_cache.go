// +build linux

package network

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/golang/groupcache/lru"
	"github.com/vishvananda/netlink"
)

type routeKey struct {
	source, dest [16]byte
	netns        uint32
	connFamily   ConnectionFamily
}

type Route struct {
	Gw      util.Address
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
}

const defaultTTL = 2 * time.Minute

type RouteCache interface {
	Get(source, dest util.Address, netns uint32) (Route, bool)
}

type Router interface {
	Route(source, dest util.Address, netns uint32) (Route, bool)
}

func NewRouteCache(size int, router Router) RouteCache {
	return &routeCache{
		cache:  lru.New(size),
		router: router,
	}
}

func (c *routeCache) Get(source, dest util.Address, netns uint32) (Route, bool) {
	c.Lock()
	defer c.Unlock()

	k := newRouteKey(source, dest, netns)
	entry, ok := c.cache.Get(k)
	if ok && time.Now().Unix() >= entry.(*routeTTL).eta {
		c.cache.Remove(k)
		ok = false
	}

	if !ok && c.router != nil {
		var r Route
		if r, ok = c.router.Route(source, dest, netns); ok {
			entry = &routeTTL{
				eta:   time.Now().Add(defaultTTL).Unix(),
				entry: r,
			}

			c.cache.Add(k, entry)
		}
	}

	if !ok {
		return Route{}, false
	}

	return entry.(*routeTTL).entry, ok
}

func newRouteKey(source, dest util.Address, netns uint32) routeKey {
	k := routeKey{netns: netns}

	copy(k.source[:], source.Bytes())
	copy(k.dest[:], dest.Bytes())

	switch len(dest.Bytes()) {
	case 4:
		k.connFamily = AFINET
	case 16:
		k.connFamily = AFINET6
	}
	return k
}

type netlinkRouter struct {
	rootNs uint64
}

func NewNetlinkRouter(procRoot string) Router {
	rootNs, err := util.GetNetNsInoFromPid(procRoot, 1)
	if err != nil {
		_ = log.Errorf("netlink gw cache backing: could not get root net ns: %s", err)
		return nil
	}

	return &netlinkRouter{rootNs: rootNs}
}

func (n *netlinkRouter) Route(source, dest util.Address, netns uint32) (Route, bool) {
	// only get routes for the root net namespace
	if uint64(netns) != n.rootNs {
		return Route{}, false
	}

	routes, err := netlink.RouteGetWithOptions(
		util.NetIPFromAddress(dest),
		&netlink.RouteGetOptions{SrcAddr: util.NetIPFromAddress(source)})

	if err == nil && len(routes) == 1 {
		r := routes[0]
		return Route{
			Gw:      util.AddressFromNetIP(r.Gw),
			IfIndex: r.LinkIndex,
		}, true
	}

	return Route{}, false
}
