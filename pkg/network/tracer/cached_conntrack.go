// +build linux_bpf

package tracer

import (
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ct "github.com/florianl/go-conntrack"
	"github.com/golang/groupcache/lru"
	"golang.org/x/sys/unix"
)

type refCounted interface {
	Incr()
	Decr()
}

type refCountedConntrack struct {
	ctrk  netlink.Conntrack
	count *uint32
}

type cachedConntrack struct {
	sync.Mutex
	procRoot         string
	cache            *lru.Cache
	conntrackCreator func(int) (netlink.Conntrack, error)
	closed           bool
}

func newCachedConntrack(procRoot string, conntrackCreator func(int) (netlink.Conntrack, error), size int) *cachedConntrack {
	cache := &cachedConntrack{
		procRoot:         procRoot,
		conntrackCreator: conntrackCreator,
		cache:            lru.New(size),
	}

	cache.cache.OnEvicted = func(_ lru.Key, v interface{}) {
		v.(netlink.Conntrack).Close()
	}

	return cache
}

func (cache *cachedConntrack) Close() error {
	cache.Lock()
	defer cache.Unlock()

	cache.cache.Clear()
	cache.closed = true
	return nil
}

func (cache *cachedConntrack) Exists(c *ConnTuple) (bool, error) {
	ctrk, err := cache.ensureConntrack(c.NetNS(), int(c.Pid()))
	if err != nil {
		return false, err
	}
	defer ctrk.Close()

	var protoNumber uint8 = unix.IPPROTO_UDP
	if c.isTCP() {
		protoNumber = unix.IPPROTO_TCP
	}

	srcAddr, dstAddr := util.NetIPFromAddress(c.SourceAddress()), util.NetIPFromAddress(c.DestAddress())
	srcPort, dstPort := c.SourcePort(), c.DestPort()

	conn := netlink.Con{
		Con: ct.Con{
			Origin: &ct.IPTuple{
				Src: &srcAddr,
				Dst: &dstAddr,
				Proto: &ct.ProtoTuple{
					Number:  &protoNumber,
					SrcPort: &srcPort,
					DstPort: &dstPort,
				},
			},
		},
	}

	ok, err := ctrk.Exists(&conn)
	if err != nil {
		log.Debugf("error while checking conntrack for connection %#v: %s", conn, err)
		cache.removeConntrack(c.NetNS())
		return false, err
	}

	if ok {
		return ok, nil
	}

	conn.Reply = conn.Origin
	conn.Origin = nil
	ok, err = ctrk.Exists(&conn)
	if err != nil {
		log.Debugf("error while checking conntrack for connection %#v: %s", conn, err)
		cache.removeConntrack(c.NetNS())
		return false, err
	}

	return ok, nil
}

func (cache *cachedConntrack) removeConntrack(ino uint64) {
	cache.Lock()
	defer cache.Unlock()

	cache.cache.Remove(ino)
}

func (cache *cachedConntrack) ensureConntrack(ino uint64, pid int) (netlink.Conntrack, error) {
	cache.Lock()
	defer cache.Unlock()

	if cache.closed {
		return nil, fmt.Errorf("cache Close has already been called")
	}

	v, ok := cache.cache.Get(ino)
	if ok {
		v.(refCounted).Incr()
		return v.(netlink.Conntrack), nil
	}

	ns, err := util.GetNetNamespaceFromPid(cache.procRoot, pid)
	if err != nil {
		log.Errorf("could not get net ns for pid %d: %s", pid, err)
		return nil, err
	}
	defer ns.Close()

	ctrk, err := cache.conntrackCreator(int(ns))
	if err != nil {
		log.Errorf("could not create conntrack object for net ns %d: %s", ino, err)
		return nil, err
	}

	r := wrapConntrack(ctrk)
	r.(refCounted).Incr() // for the caller
	cache.cache.Add(ino, r)
	return r, nil
}

func wrapConntrack(ctrk netlink.Conntrack) netlink.Conntrack {
	r := refCountedConntrack{
		ctrk:  ctrk,
		count: new(uint32),
	}
	*r.count = 1
	return r
}

func (r refCountedConntrack) Incr() {
	atomic.AddUint32(r.count, 1)
}

func (r refCountedConntrack) Decr() {
	r.decr()
}

// Exists checks if a connection exists in the conntrack
// table based on matches to `conn.Origin` or `conn.Reply`.
func (r refCountedConntrack) Exists(conn *netlink.Con) (bool, error) {
	return r.ctrk.Exists(conn)
}

// Dump dumps the conntrack table.
func (r refCountedConntrack) Dump() ([]netlink.Con, error) {
	return r.ctrk.Dump()
}

// Get gets the conntrack record for a connection. Similar to
// Exists, but returns the full connection information.
func (r refCountedConntrack) Get(conn *netlink.Con) (netlink.Con, error) {
	return r.ctrk.Get(conn)
}

// Close closes the conntrack object
func (r refCountedConntrack) Close() error {
	return r.decr()
}

func (r refCountedConntrack) decr() error {
	var err error
	if atomic.AddUint32(r.count, ^uint32(0)) == 0 {
		err = r.ctrk.Close()
	}

	return err
}
