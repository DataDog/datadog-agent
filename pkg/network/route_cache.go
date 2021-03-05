// +build linux

package network

import (
	"fmt"
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/golang/groupcache/lru"
	"github.com/vishvananda/netlink"
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

const defaultTTL = 2 * time.Minute

// RouteCache is the interface to a cache that stores routes for a given (source, destination, net ns) tuple
type RouteCache interface {
	Get(source, dest util.Address, netns uint32) (Route, bool)
}

// Router is an interface to get a route for a (source, destination, net ns) tuple
type Router interface {
	Route(source, dest util.Address, netns uint32) (Route, bool)
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

	return &routeCache{
		cache:  lru.New(size),
		router: router,
		ttl:    ttl,
	}
}

func (c *routeCache) Get(source, dest util.Address, netns uint32) (Route, bool) {
	c.Lock()
	defer c.Unlock()

	k := newRouteKey(source, dest, netns)
	if entry, ok := c.cache.Get(k); ok {
		if time.Now().Unix() < entry.(*routeTTL).eta {
			return entry.(*routeTTL).entry, ok
		}

		c.cache.Remove(k)
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

	switch len(dest.Bytes()) {
	case 4:
		k.connFamily = AFINET
	case 16:
		k.connFamily = AFINET6
	}
	return k
}

type netlinkRouter struct {
	rootNs uint32
}

// NewNetlinkRouter create a Router that queries routes via netlink
func NewNetlinkRouter(procRoot string) (Router, error) {
	rootNs, err := util.GetNetNsInoFromPid(procRoot, 1)
	if err != nil {
		return nil, fmt.Errorf("netlink gw cache backing: could not get root net ns: %w", err)
	}

	return &netlinkRouter{rootNs: rootNs}, nil
}

func (n *netlinkRouter) Route(source, dest util.Address, netns uint32) (Route, bool) {
	// only get routes for the root net namespace
	if netns != n.rootNs {
		return Route{}, false
	}

	routes, err := netlink.RouteGetWithOptions(
		util.NetIPFromAddress(dest),
		&netlink.RouteGetOptions{SrcAddr: util.NetIPFromAddress(source)})

	if err == nil && len(routes) == 1 {
		r := routes[0]
		return Route{
			Gateway: util.AddressFromNetIP(r.Gw),
			IfIndex: r.LinkIndex,
		}, true
	}

	return Route{}, false
}
