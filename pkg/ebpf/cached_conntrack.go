// +build linux_bpf

package ebpf

import (
	"sync"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	ct "github.com/florianl/go-conntrack"
	"golang.org/x/sys/unix"
)

const defaultConntrackNetlinkTTLSeconds = time.Minute
const defaultConntrackCacheShrinkInterval = time.Minute

type cachedConntrackValue struct {
	ctrk netlink.Conntrack
	eta  time.Time
}

type cachedConntrack struct {
	sync.Mutex
	procRoot         string
	conntrackByNs    map[uint64]*cachedConntrackValue
	stopper          chan interface{}
	wg               *sync.WaitGroup
	ttl              time.Duration
	conntrackCreator func(int) (netlink.Conntrack, error)
}

func newCachedConntrack(procRoot string, conntrackCreator func(int) (netlink.Conntrack, error), shrinkInterval, conntrackTTL time.Duration) *cachedConntrack {
	cache := &cachedConntrack{
		procRoot:         procRoot,
		conntrackByNs:    make(map[uint64]*cachedConntrackValue),
		stopper:          make(chan interface{}),
		wg:               &sync.WaitGroup{},
		conntrackCreator: conntrackCreator,
	}

	cache.wg.Add(1)
	go func() {
		defer cache.wg.Done()

		ticker := time.NewTicker(shrinkInterval)
		defer ticker.Stop()

		for {
			select {
			case <-cache.stopper:
				return
			case <-ticker.C:
				cache.shrink()
			}
		}
	}()

	return cache
}

func (cache *cachedConntrack) shrink() {
	cache.Lock()
	defer cache.Unlock()

	copied := make(map[uint64]*cachedConntrackValue)
	now := time.Now()
	for k, v := range cache.conntrackByNs {
		if now.Before(v.eta) {
			copied[k] = v
		} else {
			v.ctrk.Close()
		}
	}

	log.Debugf("removed %d netlink connections", len(cache.conntrackByNs)-len(copied))
	cache.conntrackByNs = copied
}

func (cache *cachedConntrack) Close() error {
	func() {
		cache.Lock()
		defer cache.Unlock()

		for _, ctrk := range cache.conntrackByNs {
			ctrk.ctrk.Close()
		}
	}()

	close(cache.stopper)
	cache.wg.Wait()
	return nil
}

func (cache *cachedConntrack) Exists(c *ConnTuple) (bool, error) {
	ctrk, err := cache.ensureConntrack(c.NetNS(), int(c.Pid()))
	if err != nil {
		return false, err
	}

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
		log.Errorf("error while checking conntrack for connection %#v: %s", conn, err)
		return false, err
	}

	if ok {
		return ok, nil
	}

	conn.Reply = conn.Origin
	conn.Origin = nil
	ok, err = ctrk.Exists(&conn)
	if err != nil {
		log.Errorf("error while checking conntrack for connection %#v: %s", conn, err)
		return false, err
	}

	return ok, nil
}

func (cache *cachedConntrack) ensureConntrack(ino uint64, pid int) (netlink.Conntrack, error) {
	cache.Lock()
	defer cache.Unlock()

	if ctrk, ok := cache.conntrackByNs[ino]; ok {
		// renew the ttl
		ctrk.eta = time.Now().Add(cache.ttl)
		return ctrk.ctrk, nil
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

	cache.conntrackByNs[ino] = &cachedConntrackValue{
		ctrk: ctrk,
		eta:  time.Now().Add(cache.ttl),
	}

	return ctrk, nil
}
