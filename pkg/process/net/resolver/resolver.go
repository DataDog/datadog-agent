package resolver

import (
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/process"
	procutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/containers"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultTTL = 10 * time.Second

// LocalResolver is responsible resolving the raddr of connections when they are local containers
type LocalResolver struct {
	mux         sync.RWMutex
	addrToCtrID map[model.ContainerAddr]string
	ctrForPid   map[int32]string
	updated     time.Time
}

type addrWithNS struct {
	model.ContainerAddr
	netns uint32
}

// LoadAddrs generates a map of network addresses to container IDs
func (l *LocalResolver) LoadAddrs(containers []*containers.Container) {
	l.mux.Lock()
	defer l.mux.Unlock()

	if time.Now().Sub(l.updated) < defaultTTL {
		return
	}

	l.updated = time.Now()
	l.addrToCtrID = make(map[model.ContainerAddr]string)
	l.ctrForPid = make(map[int32]string)
	for _, ctr := range containers {
		for _, pid := range ctr.Pids {
			l.ctrForPid[pid] = ctr.ID
		}

		for _, networkAddr := range ctr.AddressList {
			if networkAddr.IP.IsLoopback() {
				continue
			}
			addr := model.ContainerAddr{
				Ip:       networkAddr.IP.String(),
				Port:     int32(networkAddr.Port),
				Protocol: model.ConnectionType(model.ConnectionType_value[networkAddr.Protocol]),
			}
			l.addrToCtrID[addr] = ctr.ID
		}

	}
}

// Resolve binds container IDs to the Raddr of connections
//
// An attempt is made to resolve as many local containers as possible.
//
// First, we go over all connections resolving the laddr container
// using the pid to container map that we have. At the same time,
// the translated laddr is put into a lookup table (addr -> container id),
// qualifying the key in that table with the network namespace id
// if the address is loopback.
//
// Second, we go over the connections again, this time resolving
// the raddr container id using the lookup table we built previously.
// Note that the translated raddr is *not* used for the lookup.  For
// loopback addresses, lookup is qualified by the network namespace
// they are in.
func (l *LocalResolver) Resolve(c *model.Connections) {
	l.mux.RLock()
	defer l.mux.RUnlock()

	// hash used for loopback resolution
	ctrsByLaddr := make(map[addrWithNS]string, len(c.Conns))

	for _, conn := range c.Conns {
		raddr := conn.Raddr

		addr := model.ContainerAddr{
			Ip:       raddr.Ip,
			Port:     raddr.Port,
			Protocol: conn.Type,
		}

		raddr.ContainerId = l.addrToCtrID[addr]

		// resolve laddr
		cid, ok := l.ctrForPid[conn.Pid]
		if !ok {
			continue
		}

		conn.Laddr.ContainerId = cid

		ip := procutil.AddressFromString(conn.Laddr.Ip)
		if ip == nil {
			continue
		}

		laddr := model.ContainerAddr{
			Ip:       conn.Laddr.Ip,
			Port:     conn.Laddr.Port,
			Protocol: conn.Type,
		}

		netns := conn.NetNS
		ctrsByLaddr[addrWithNS{laddr, netns}] = cid
		if !ip.IsLoopback() {
			ctrsByLaddr[addrWithNS{laddr, 0}] = cid
		}
	}

	log.Tracef("ctrsByLaddr = %v", ctrsByLaddr)

	// go over connections again using hashtable computed earlier to resolver raddr
	for _, conn := range c.Conns {
		if conn.Raddr.ContainerId == "" {
			raddr := translatedContainerRaddr(conn.Raddr, conn.IpTranslation, conn.Type)
			ip := procutil.AddressFromString(raddr.Ip)
			if ip == nil {
				continue
			}

			// first match within net namespace
			cid, ok := ctrsByLaddr[addrWithNS{raddr, conn.NetNS}]
			if !ok && !ip.IsLoopback() {
				cid, _ = ctrsByLaddr[addrWithNS{raddr, 0}]
			}

			conn.Raddr.ContainerId = cid
		}

		if conn.Raddr.ContainerId == "" {
			log.Tracef("could not resolve raddr %v", conn.Raddr)
		}
	}
}

func translatedContainerRaddr(raddr *model.Addr, trans *model.IPTranslation, proto model.ConnectionType) model.ContainerAddr {
	if trans == nil {
		return model.ContainerAddr{
			Ip:       raddr.Ip,
			Port:     raddr.Port,
			Protocol: proto,
		}
	}

	return model.ContainerAddr{
		Ip:       trans.ReplSrcIP,
		Port:     trans.ReplSrcPort,
		Protocol: proto,
	}
}
