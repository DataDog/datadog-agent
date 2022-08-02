// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package resolver

import (
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	procutil "github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const defaultTTL = 10 * time.Second

// LocalResolver is responsible resolving the raddr of connections when they are local containers
type LocalResolver struct {
	mux         sync.RWMutex
	addrToCtrID map[model.ContainerAddr]string
	ctrForPid   map[int]string
	updated     time.Time
}

type addrWithNS struct {
	model.ContainerAddr
	netns uint32
}

// LoadAddrs generates a map of network addresses to container IDs
func (l *LocalResolver) LoadAddrs(containers []*model.Container, pidToCid map[int]string) {
	l.mux.Lock()
	defer l.mux.Unlock()

	if time.Now().Sub(l.updated) < defaultTTL {
		return
	}

	l.updated = time.Now()
	l.addrToCtrID = make(map[model.ContainerAddr]string)
	l.ctrForPid = pidToCid
	for _, ctr := range containers {
		for _, networkAddr := range ctr.Addresses {
			parsedAddr := procutil.AddressFromString(networkAddr.Ip)
			if parsedAddr.IsLoopback() {
				continue
			}
			l.addrToCtrID[*networkAddr] = ctr.Id
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
		cid, ok := l.ctrForPid[int(conn.Pid)]
		if !ok {
			continue
		}
		conn.Laddr.ContainerId = cid

		ip := procutil.AddressFromString(conn.Laddr.Ip)
		if ip.IsZero() {
			continue
		}

		laddr := model.ContainerAddr{
			Ip:       conn.Laddr.Ip,
			Port:     conn.Laddr.Port,
			Protocol: conn.Type,
		}
		if conn.NetNS != 0 {
			ctrsByLaddr[addrWithNS{laddr, conn.NetNS}] = cid
		}
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
			if ip.IsZero() {
				continue
			}

			// first match within net namespace
			var ok bool
			if conn.NetNS != 0 {
				if conn.Raddr.ContainerId, ok = ctrsByLaddr[addrWithNS{raddr, conn.NetNS}]; ok {
					continue
				}
			}
			if !ip.IsLoopback() {
				if conn.Raddr.ContainerId, ok = ctrsByLaddr[addrWithNS{raddr, 0}]; ok {
					continue
				}
			}
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
