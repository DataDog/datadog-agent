// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//nolint:revive // TODO(PROC) Fix revive linter
package resolver

import (
	"fmt"
	"net/netip"
	"sync"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/benbjohnson/clock"
	"go4.org/intern"

	procutil "github.com/DataDog/datadog-agent/pkg/process/util"
	proccontainers "github.com/DataDog/datadog-agent/pkg/process/util/containers"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	cacheValidityNoRT = 2 * time.Second
)

var resolverTelemetry = struct {
	cacheSize  telemetry.Gauge
	cacheDrops telemetry.Counter
}{
	telemetry.NewGauge("net_local_resolver", "cache_size", []string{"cache"}, "Gauge for cache sizes"),
	telemetry.NewCounter("net_local_resolver", "cache_drops", []string{"cache"}, "Gauge for cache drops"),
}

type containerIDEntry struct {
	cid   *intern.Value
	inUse bool
}

// LocalResolver is responsible resolving the raddr of connections when they are local containers
type LocalResolver struct {
	mux                sync.Mutex
	addrToCtrID        map[model.ContainerAddr]*containerIDEntry
	maxAddrToCtrIDSize int
	ctrForPid          map[int]*containerIDEntry
	maxCtrForPidSize   int
	lastContainerRates map[string]*proccontainers.ContainerRateMetrics
	Clock              clock.Clock
	ContainerProvider  proccontainers.ContainerProvider
	done               chan bool
}

func NewLocalResolver(containerProvider proccontainers.ContainerProvider, clock clock.Clock, maxAddrCacheSize, maxPidCacheSize int) *LocalResolver {
	return &LocalResolver{
		ContainerProvider:  containerProvider,
		Clock:              clock,
		done:               make(chan bool),
		addrToCtrID:        make(map[model.ContainerAddr]*containerIDEntry),
		maxAddrToCtrIDSize: maxAddrCacheSize,
		ctrForPid:          make(map[int]*containerIDEntry),
		maxCtrForPidSize:   maxPidCacheSize,
	}
}

func (l *LocalResolver) Run() {
	pullContainerFrequency := 10 * time.Second
	ticker := l.Clock.Ticker(pullContainerFrequency)
	go l.pullContainers(ticker)
}

func (l *LocalResolver) Stop() {
	l.done <- true
}

func (l *LocalResolver) pullContainers(ticker *clock.Ticker) {
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			containers, lastContainerRates, pidToCid, err := l.ContainerProvider.GetContainers(cacheValidityNoRT, l.lastContainerRates)
			if err == nil {
				l.lastContainerRates = lastContainerRates
			} else {
				log.Warnf("Unable to gather stats for containers, err: %v", err)
			}

			// Keep track of containers addresses
			l.LoadAddrs(containers, pidToCid)

		case <-l.done:
			return
		}
	}
}

// LoadAddrs generates a map of network addresses to container IDs
func (l *LocalResolver) LoadAddrs(containers []*model.Container, pidToCid map[int]string) {
	l.mux.Lock()
	defer l.mux.Unlock()

	// mark everything not in use
	for _, c := range l.addrToCtrID {
		c.inUse = false
	}
	for _, c := range l.ctrForPid {
		c.inUse = false
	}

containersLoop:
	for _, ctr := range containers {
		for _, networkAddr := range ctr.Addresses {
			if len(l.addrToCtrID) >= l.maxAddrToCtrIDSize {
				log.Warnf("address to container ID cache has reached max size of %d entries", l.maxAddrToCtrIDSize)
				resolverTelemetry.cacheDrops.Inc("addr_cache")
				break containersLoop
			}

			parsedAddr := procutil.AddressFromString(networkAddr.Ip)
			if parsedAddr.IsLoopback() {
				continue
			}
			l.addrToCtrID[*networkAddr] = &containerIDEntry{
				cid:   intern.GetByString(ctr.Id),
				inUse: true,
			}
		}
	}

	resolverTelemetry.cacheSize.Set(float64(len(l.addrToCtrID)), "addr_cache")

	for pid, cid := range pidToCid {
		if len(l.ctrForPid) >= l.maxCtrForPidSize {
			log.Warnf("pid to container ID cache has reached max size of %d entries", l.maxCtrForPidSize)
			resolverTelemetry.cacheDrops.Inc("pid_cache")
			break
		}

		l.ctrForPid[pid] = &containerIDEntry{
			cid:   intern.GetByString(cid),
			inUse: true,
		}
	}

	resolverTelemetry.cacheSize.Set(float64(len(l.ctrForPid)), "pid_cache")
}

// Resolve binds container IDs to the Raddr of connections
//
// An attempt is made to resolve as many local containers as possible.
//
// First, we go over all connections resolving the laddr container
// using the pid to container map that we have. We also index
// connections by (laddr, raddr, proto, netns), inserting a
// non-loopback saddr with netns = 0 as well. Translated
// laddr and raddr are used throughout.
//
// Second, we go over the connections again, this time resolving
// the raddr container id using the lookup table we built previously.
// Translated addresses are used throughout.
//
// Only connections that are local are resolved, i.e., for
// which conn.IntrHost is set to true.
//
// If lookup by table fails above, we fall back to using
// the l.addrToCtrID map
func (l *LocalResolver) Resolve(c *model.Connections) {
	l.mux.Lock()
	defer l.mux.Unlock()

	defer func() {
		// remove all not in use entries
		for pid, ctr := range l.ctrForPid {
			if !ctr.inUse {
				delete(l.ctrForPid, pid)
			}
		}
		for addr, ctr := range l.addrToCtrID {
			if !ctr.inUse {
				delete(l.addrToCtrID, addr)
			}
		}

		resolverTelemetry.cacheSize.Set(float64(len(l.ctrForPid)), "pid_cache")
		resolverTelemetry.cacheSize.Set(float64(len(l.addrToCtrID)), "addr_cache")
	}()

	type connKey struct {
		laddr, raddr netip.AddrPort
		proto        model.ConnectionType
		netns        uint32
	}

	ctrsByConn := make(map[connKey]string, len(c.Conns))
	for _, conn := range c.Conns {
		if conn.Laddr == nil {
			continue
		}

		// resolve laddr
		//
		// if process monitoring is enabled in the system-probe,
		// then laddr container id may be set, so check that
		// first
		cid := conn.Laddr.ContainerId
		if cid == "" {
			if v, ok := l.ctrForPid[int(conn.Pid)]; ok {
				cid = v.cid.Get().(string)
			}
		}

		if cid == "" {
			continue
		}

		conn.Laddr.ContainerId = cid

		if !conn.IntraHost {
			continue
		}

		laddr, raddr, err := translatedAddrs(conn)
		if err != nil {
			log.Error(err)
			continue
		}

		if conn.Direction == model.ConnectionDirection_incoming {
			raddr = netip.AddrPortFrom(raddr.Addr(), 0)
		}

		k := connKey{
			laddr: laddr,
			raddr: raddr,
			proto: conn.Type,
			netns: conn.NetNS,
		}
		if conn.NetNS != 0 {
			ctrsByConn[k] = cid
		}
		if !laddr.Addr().IsLoopback() {
			k.netns = 0
			ctrsByConn[k] = cid
		}
	}

	log.Tracef("ctrsByConn = %v", ctrsByConn)

	// go over connections again using hashtable computed earlier to resolver raddr
	for _, conn := range c.Conns {
		if conn.Raddr.ContainerId != "" {
			continue
		}

		laddr, raddr, err := translatedAddrs(conn)
		if err != nil {
			log.Error(err)
			continue
		}

		if conn.IntraHost {
			if conn.Direction == model.ConnectionDirection_outgoing {
				laddr = netip.AddrPortFrom(laddr.Addr(), 0)
			}

			var ok bool
			k := connKey{
				laddr: raddr,
				raddr: laddr,
				proto: conn.Type,
				netns: conn.NetNS,
			}
			if conn.Raddr.ContainerId, ok = ctrsByConn[k]; ok {
				continue
			}

			if !raddr.Addr().IsLoopback() {
				k.netns = 0
				if conn.Raddr.ContainerId, ok = ctrsByConn[k]; ok {
					continue
				}
			}
		}

		if v, ok := l.addrToCtrID[model.ContainerAddr{
			Ip:       raddr.Addr().String(),
			Port:     int32(raddr.Port()),
			Protocol: conn.Type,
		}]; ok {
			conn.Raddr.ContainerId = v.cid.Get().(string)
		} else {
			log.Tracef("could not resolve raddr %v", conn.Raddr)
		}
	}
}

func parseAddrPort(ip string, port uint16) (netip.AddrPort, error) {
	addr, err := netip.ParseAddr(ip)
	if err != nil {
		return netip.AddrPort{}, err
	}

	if !addr.IsValid() || addr.IsUnspecified() {
		return netip.AddrPort{}, fmt.Errorf("invalid or unspecified address: %+v", ip)
	}

	return netip.AddrPortFrom(addr, port), nil
}

func translatedRaddr(raddr *model.Addr, trans *model.IPTranslation) (netip.AddrPort, error) {
	ip := raddr.Ip
	port := raddr.Port
	if trans != nil {
		ip = trans.ReplSrcIP
		port = trans.ReplSrcPort
	}

	return parseAddrPort(ip, uint16(port))
}

func translatedLaddr(laddr *model.Addr, trans *model.IPTranslation) (netip.AddrPort, error) {
	ip := laddr.Ip
	port := laddr.Port
	if trans != nil {
		ip = trans.ReplDstIP
		port = trans.ReplDstPort
	}

	return parseAddrPort(ip, uint16(port))
}

func translatedAddrs(conn *model.Connection) (laddr, raddr netip.AddrPort, err error) {
	if conn.Laddr != nil {
		laddr, err = translatedLaddr(conn.Laddr, conn.IpTranslation)
		if err != nil {
			return laddr, raddr, err
		}
	}

	raddr, err = translatedRaddr(conn.Raddr, conn.IpTranslation)
	return laddr, raddr, err
}
