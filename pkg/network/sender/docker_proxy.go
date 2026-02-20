// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"maps"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"

	log "github.com/DataDog/datadog-agent/comp/core/log/def"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
	"github.com/DataDog/datadog-agent/pkg/telemetry"
	ddmaps "github.com/DataDog/datadog-agent/pkg/util/maps"
	ddos "github.com/DataDog/datadog-agent/pkg/util/os"
)

const dockerProxySubsystem = "sender__docker_proxy"

var dockerProxyTelemetry = struct {
	processesDetected   telemetry.Counter
	ipsFound            telemetry.Counter
	connectionsFiltered telemetry.Counter
	activeProxies       telemetry.Gauge
}{
	telemetry.NewCounter(dockerProxySubsystem, "processes_detected", nil, ""),
	telemetry.NewCounter(dockerProxySubsystem, "ips_found", nil, ""),
	telemetry.NewCounter(dockerProxySubsystem, "connections_filtered", nil, ""),
	telemetry.NewGauge(dockerProxySubsystem, "active_proxies", nil, ""),
}

type containerAddr struct {
	addr  netip.AddrPort
	proto network.ConnectionType
}

type proxy struct {
	pid    uint32
	ip     netip.Addr
	target containerAddr
	alive  bool
}

type dockerProxyFilter struct {
	log log.Component

	mtx           sync.Mutex
	proxyByTarget map[containerAddr]*proxy
	proxyByPID    map[uint32]*proxy

	pidAliveFunc func(pid int) bool
}

func newDockerProxyFilter(log log.Component) *dockerProxyFilter {
	return &dockerProxyFilter{
		log:           log,
		proxyByTarget: make(map[containerAddr]*proxy),
		proxyByPID:    make(map[uint32]*proxy),
		pidAliveFunc:  ddos.PidExists,
	}
}

func (d *dockerProxyFilter) process(event *process) {
	d.mtx.Lock()
	defer d.mtx.Unlock()

	// TODO if we miss the fork/exec event, we will never consider that process a docker-proxy

	if proxy, seen := d.proxyByPID[event.Pid]; seen {
		if event.EventType == model.ExitEventType {
			// mark proxy as dead so it will be removed after next set of connections are filtered
			proxy.alive = false
			return
		}
		if event.EventType != model.ExecEventType && event.EventType != model.ForkEventType {
			return
		}
		// we've received a new exec event with the same PID as an existing entry.
		// mark the previous one as dead and remove it from the pid->proxy map.
		// the target map will be cleaned up by marking it as dead
		proxy.alive = false
		delete(d.proxyByPID, event.Pid)
	}

	if proxy := extractProxyTarget(event); proxy != nil {
		dockerProxyTelemetry.processesDetected.Inc()
		d.log.Debugf("detected docker-proxy with pid=%d target.ip=%s target.port=%d target.proto=%s",
			proxy.pid,
			proxy.target.addr.Addr(),
			proxy.target.addr.Port(),
			proxy.target.proto,
		)

		d.proxyByPID[proxy.pid] = proxy
		d.proxyByTarget[proxy.target] = proxy
		dockerProxyTelemetry.activeProxies.Set(float64(len(d.proxyByTarget)))
	}
}

// FilterProxies removes all connections that are related to docker-proxy processes
func (d *dockerProxyFilter) FilterProxies(conns *network.Connections) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	if len(d.proxyByTarget) == 0 {
		return
	}

	undiscoveredProxies := ddmaps.Filter(d.proxyByPID, func(_ uint32, p *proxy) bool {
		return !p.ip.IsValid()
	})
	for _, conn := range conns.Conns {
		// early break if we've already discovered all the proxies
		if len(undiscoveredProxies) == 0 {
			break
		}

		if proxy, ok := undiscoveredProxies[conn.Pid]; ok {
			if proxyIP := d.discoverProxyIP(proxy, conn); proxyIP.IsValid() {
				dockerProxyTelemetry.ipsFound.Inc()
				proxy.ip = proxyIP
				delete(undiscoveredProxies, conn.Pid)
			}
		}
	}

	conns.Conns = slices.DeleteFunc(conns.Conns, func(c network.ConnectionStats) bool {
		proxied := d.isProxied(c)
		if proxied {
			dockerProxyTelemetry.connectionsFiltered.Inc()
		}
		return proxied
	})

	for pid, p := range d.proxyByPID {
		// if already marked dead, no reason to re-interrogate
		if p.alive {
			p.alive = d.pidAliveFunc(int(pid))
		}
	}
	maps.DeleteFunc(d.proxyByPID, func(_ uint32, p *proxy) bool { return !p.alive })
	maps.DeleteFunc(d.proxyByTarget, func(_ containerAddr, p *proxy) bool { return !p.alive })
	dockerProxyTelemetry.activeProxies.Set(float64(len(d.proxyByTarget)))
}

func (d *dockerProxyFilter) isProxied(c network.ConnectionStats) bool {
	if p, ok := d.proxyByTarget[containerAddr{addr: netip.AddrPortFrom(c.Source.Addr, c.SPort), proto: c.Type}]; ok {
		return p.ip == c.Dest.Addr
	}
	if p, ok := d.proxyByTarget[containerAddr{addr: netip.AddrPortFrom(c.Dest.Addr, c.DPort), proto: c.Type}]; ok {
		return p.ip == c.Source.Addr
	}
	return false
}

func (d *dockerProxyFilter) discoverProxyIP(p *proxy, c network.ConnectionStats) netip.Addr {
	// The heuristic here goes as follows:
	// One of the ends of this connections must match p.target;
	// The proxy IP will be the other end;
	if netip.AddrPortFrom(c.Source.Addr, c.SPort) == p.target.addr {
		return c.Dest.Addr
	}
	if netip.AddrPortFrom(c.Dest.Addr, c.DPort) == p.target.addr {
		return c.Source.Addr
	}

	return netip.Addr{}
}

func extractProxyTarget(p *process) *proxy {
	if len(p.Cmdline) == 0 {
		return nil
	}
	cmd := p.Cmdline
	if !strings.HasSuffix(cmd[0], "docker-proxy") {
		return nil
	}

	// Extract proxy target address
	proxy := &proxy{pid: p.Pid, alive: true}
	var ip netip.Addr
	var port uint16
	var err error
	// len(cmd)-1 because the value of the argument will be next
	for i := 1; i < len(cmd)-1; i++ {
		switch cmd[i] {
		case "-container-ip":
			ip, err = netip.ParseAddr(cmd[i+1])
			if err != nil {
				return nil
			}
			i++
		case "-container-port":
			port64, err := strconv.ParseUint(cmd[i+1], 10, 16)
			if err != nil {
				return nil
			}
			port = uint16(port64)
			i++
		case "-proto":
			name := cmd[i+1]
			proto, ok := network.ConnectionTypeFromString[strings.ToLower(name)]
			if !ok {
				return nil
			}
			proxy.target.proto = proto
			i++
		}
	}

	if !ip.IsValid() || port == 0 {
		return nil
	}
	proxy.target.addr = netip.AddrPortFrom(ip, port)
	return proxy
}
