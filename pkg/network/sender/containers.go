// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"context"
	"fmt"
	"maps"
	"net/netip"
	"sync"
	"time"

	"go4.org/intern"

	workloadmeta "github.com/DataDog/datadog-agent/comp/core/workloadmeta/def"
	pkgconfigsetup "github.com/DataDog/datadog-agent/pkg/config/setup"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const containerResolverSubsystem = "sender__container_resolver"

var containerResolverTelemetry = struct {
	addressCount telemetry.Gauge
	tagCount     telemetry.Gauge
}{
	telemetry.NewGauge(containerResolverSubsystem, "address_count", nil, ""),
	telemetry.NewGauge(containerResolverSubsystem, "tag_count", nil, ""),
}

type connKey struct {
	laddr, raddr netip.AddrPort
	proto        network.ConnectionType
	netns        uint32
}

type containerID struct {
	id    *intern.Value
	alive bool
}

type containerResolver struct {
	wmeta workloadmeta.Component

	mtx               sync.Mutex
	addrToContainerID map[containerAddr]containerID
	tagContainerIDs   map[containerID]struct{}
	tagDuration       time.Duration
}

func newContainerResolver(wmeta workloadmeta.Component, tagDuration time.Duration) *containerResolver {
	cr := &containerResolver{
		wmeta:             wmeta,
		addrToContainerID: make(map[containerAddr]containerID),
	}
	if tagDuration > 0 {
		cr.tagContainerIDs = make(map[containerID]struct{})
		cr.tagDuration = tagDuration
	}
	return cr
}

func (r *containerResolver) start(ctx context.Context) {
	filter := workloadmeta.NewFilterBuilder().
		AddKind(workloadmeta.KindContainer).
		SetEventType(workloadmeta.EventTypeAll).
		Build()
	ch := r.wmeta.Subscribe("CNM Container Resolver", workloadmeta.NormalPriority, filter)
	go func() {
		for {
			select {
			case <-ctx.Done():
				r.wmeta.Unsubscribe(ch)
				return
			case eventBundle, ok := <-ch:
				if !ok {
					return
				}

				r.process(eventBundle)
				eventBundle.Acknowledge()
			}
		}
	}()
}

func (r *containerResolver) process(eventBundle workloadmeta.EventBundle) {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	for _, event := range eventBundle.Events {
		container, ok := event.Entity.(*workloadmeta.Container)
		if !ok {
			continue
		}

		idWrapper := containerID{
			id: intern.GetByString(container.ID),
			// containers flagged as not alive will be cleared on next resolution pass
			alive: event.Type == workloadmeta.EventTypeSet,
		}

		// map ip+port to container ID
		for _, addr := range container.NetworkIPs {
			netaddr, err := netip.ParseAddr(addr)
			if err != nil || netaddr.IsLoopback() {
				continue
			}
			for _, port := range container.Ports {
				r.addrToContainerID[containerAddr{
					addr:  netip.AddrPortFrom(netaddr, uint16(port.Port)),
					proto: network.ConnectionTypeFromString[port.Protocol],
				}] = idWrapper
			}
		}

		// see if container qualifies for tagging
		if r.tagDuration > 0 {
			currentTime := time.Now()
			withinAgentStartingPeriod := pkgconfigsetup.StartTime.Add(r.tagDuration).After(currentTime)
			if withinAgentStartingPeriod || container.State.StartedAt.Add(r.tagDuration).After(currentTime) {
				if !idWrapper.alive {
					k := containerID{id: idWrapper.id, alive: true}
					delete(r.tagContainerIDs, k)
				}
				r.tagContainerIDs[idWrapper] = struct{}{}
			}
		}
	}

	containerResolverTelemetry.addressCount.Set(float64(len(r.addrToContainerID)))
	containerResolverTelemetry.tagCount.Set(float64(len(r.tagContainerIDs)))
}

func (r *containerResolver) shouldTagContainer(cid *intern.Value) bool {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	if r.tagDuration > 0 {
		if _, ok := r.tagContainerIDs[containerID{id: cid, alive: true}]; ok {
			return true
		}
		if _, ok := r.tagContainerIDs[containerID{id: cid, alive: false}]; ok {
			return true
		}
	}
	return false
}

func (r *containerResolver) removeDeadTagContainers() {
	r.mtx.Lock()
	defer r.mtx.Unlock()

	maps.DeleteFunc(r.tagContainerIDs, func(id containerID, _ struct{}) bool {
		return !id.alive
	})
	containerResolverTelemetry.tagCount.Set(float64(len(r.tagContainerIDs)))
}

func (r *containerResolver) resolveDestinationContainerIDs(conns *network.Connections) {
	r.mtx.Lock()
	defer r.mtx.Unlock()
	defer func() {
		maps.DeleteFunc(r.addrToContainerID, func(_ containerAddr, id containerID) bool {
			return !id.alive
		})
		containerResolverTelemetry.addressCount.Set(float64(len(r.addrToContainerID)))
	}()

	// establish a tighter bound on a number of container IDs for the map
	containerIDCount := 0
	for i := range conns.Conns {
		conn := &conns.Conns[i]
		if !conn.IntraHost {
			continue
		}
		cid := conn.ContainerID.Source
		if cid == nil {
			continue
		}
		conn.ContainerID.Source = cid

		if conn.NetNS != 0 {
			containerIDCount++
			continue
		}

		laddr := conn.Source.Addr
		if conn.IPTranslation != nil {
			laddr = conn.IPTranslation.ReplDstIP.Addr
		}
		if !laddr.IsLoopback() {
			containerIDCount++
			continue
		}
	}

	containerIDByConnection := make(map[connKey]*intern.Value, containerIDCount)
	for i := range conns.Conns {
		conn := &conns.Conns[i]
		if !conn.IntraHost {
			continue
		}
		cid := conn.ContainerID.Source
		if cid == nil {
			continue
		}
		laddr, raddr, err := translatedAddrs(conn)
		if err != nil {
			log.Error(err)
			continue
		}
		if conn.Direction == network.INCOMING {
			raddr = netip.AddrPortFrom(raddr.Addr(), 0)
		}

		k := connKey{
			laddr: laddr,
			raddr: raddr,
			proto: conn.Type,
			netns: conn.NetNS,
		}
		if k.netns != 0 {
			containerIDByConnection[k] = cid
		}
		if !laddr.Addr().IsLoopback() {
			k.netns = 0
			containerIDByConnection[k] = cid
		}
	}

	if log.ShouldLog(log.TraceLvl) {
		log.Tracef("containerIDByConnection = %v", containerIDByConnection)
	}

	// go over connections again using hashtable computed earlier to containerResolver raddr
	for i := range conns.Conns {
		conn := &conns.Conns[i]
		if conn.ContainerID.Dest != nil {
			continue
		}

		laddr, raddr, err := translatedAddrs(conn)
		if err != nil {
			log.Error(err)
			continue
		}

		if conn.IntraHost {
			if conn.Direction == network.OUTGOING {
				laddr = netip.AddrPortFrom(laddr.Addr(), 0)
			}
		}
		var ok bool
		k := connKey{
			laddr: raddr,
			raddr: laddr,
			proto: conn.Type,
			netns: conn.NetNS,
		}
		if conn.ContainerID.Dest, ok = containerIDByConnection[k]; ok {
			continue
		}
		if !raddr.Addr().IsLoopback() {
			k.netns = 0
			if conn.ContainerID.Dest, ok = containerIDByConnection[k]; ok {
				continue
			}
		}

		if v, ok := r.addrToContainerID[containerAddr{addr: raddr, proto: conn.Type}]; ok {
			conn.ContainerID.Dest = v.id
		} else {
			if log.ShouldLog(log.TraceLvl) {
				log.Tracef("could not resolve raddr %v", raddr)
			}
		}
	}
}

func translatedRaddr(ip netip.Addr, port uint16, trans *network.IPTranslation) (netip.AddrPort, error) {
	if trans != nil {
		ip = trans.ReplSrcIP.Addr
		port = trans.ReplSrcPort
	}

	if !ip.IsValid() || ip.IsUnspecified() {
		return netip.AddrPort{}, fmt.Errorf("invalid or unspecified address: %+v", ip)
	}
	return netip.AddrPortFrom(ip, port), nil
}

func translatedLaddr(ip netip.Addr, port uint16, trans *network.IPTranslation) (netip.AddrPort, error) {
	if trans != nil {
		ip = trans.ReplDstIP.Addr
		port = trans.ReplDstPort
	}

	if !ip.IsValid() || ip.IsUnspecified() {
		return netip.AddrPort{}, fmt.Errorf("invalid or unspecified address: %+v", ip)
	}

	return netip.AddrPortFrom(ip, port), nil
}

func translatedAddrs(conn *network.ConnectionStats) (laddr, raddr netip.AddrPort, err error) {
	laddr, err = translatedLaddr(conn.Source.Addr, conn.SPort, conn.IPTranslation)
	if err != nil {
		return laddr, raddr, err
	}

	raddr, err = translatedRaddr(conn.Dest.Addr, conn.DPort, conn.IPTranslation)
	return laddr, raddr, err
}
