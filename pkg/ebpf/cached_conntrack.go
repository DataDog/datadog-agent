// +build linux_bpf

package ebpf

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
		v.(refCountedConntrack).Decr()
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
	defer ctrk.Decr()

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

	ok, err := ctrk.ctrk.Exists(&conn)
	if err != nil {
		log.Errorf("error while checking conntrack for connection %#v: %s", conn, err)
		cache.cache.Remove(c.NetNS())
		return false, err
	}

	if ok {
		return ok, nil
	}

	conn.Reply = conn.Origin
	conn.Origin = nil
	ok, err = ctrk.ctrk.Exists(&conn)
	if err != nil {
		log.Errorf("error while checking conntrack for connection %#v: %s", conn, err)
		cache.cache.Remove(c.NetNS())
		return false, err
	}

	return ok, nil
}

func (cache *cachedConntrack) ensureConntrack(ino uint64, pid int) (refCountedConntrack, error) {
	cache.Lock()
	defer cache.Unlock()

	if cache.closed {
		return refCountedConntrack{}, fmt.Errorf("cache Close has already been called")
	}

	v, ok := cache.cache.Get(ino)
	if ok {
		r := v.(refCountedConntrack)
		r.Incr()
		return r, nil
	}

	ns, err := util.GetNetNamespaceFromPid(cache.procRoot, pid)
	if err != nil {
		log.Errorf("could not get net ns for pid %d: %s", pid, err)
		return refCountedConntrack{}, err
	}
	defer ns.Close()

	ctrk, err := cache.conntrackCreator(int(ns))
	if err != nil {
		log.Errorf("could not create conntrack object for net ns %d: %s", ino, err)
		return refCountedConntrack{}, err
	}

	r := wrapConntrack(ctrk)
	r.Incr() // for the caller
	cache.cache.Add(ino, r)
	return r, nil
}

func wrapConntrack(ctrk netlink.Conntrack) refCountedConntrack {
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
	if atomic.AddUint32(r.count, ^uint32(0)) == 0 {
		r.ctrk.Close()
	}
}
