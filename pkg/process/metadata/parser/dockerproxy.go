// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package parser

import (
	"strconv"
	"strings"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/process/procutil"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

// DockerProxy keeps track of every docker-proxy instance and filters network traffic going through them
type DockerProxy struct {
	proxyByTarget map[model.ContainerAddr]*proxy
	// This "secondary index" is used only during the proxy IP discovery process
	proxyByPID map[int32]*proxy
}

type proxy struct {
	pid    int32
	ip     string
	target model.ContainerAddr
}

// NewDockerProxy instantiates a new filter loaded with docker-proxy instance information
func NewDockerProxy() *DockerProxy {
	return &DockerProxy{
		proxyByPID:    make(map[int32]*proxy),
		proxyByTarget: make(map[model.ContainerAddr]*proxy),
	}
}

func (d *DockerProxy) Extract(processes map[int32]*procutil.Process) {
	proxyByPID := make(map[int32]*proxy)
	proxyByTarget := make(map[model.ContainerAddr]*proxy)

	for _, p := range processes {
		if proxy, seen := d.proxyByPID[p.Pid]; seen {
			proxyByPID[p.Pid] = proxy
			proxyByTarget[proxy.target] = proxy
			continue
		}

		if proxy := extractProxyTarget(p); proxy != nil {
			log.Debugf("detected docker-proxy with pid=%d target.ip=%s target.port=%d target.proto=%s",
				proxy.pid,
				proxy.target.Ip,
				proxy.target.Port,
				proxy.target.Protocol,
			)

			// Add proxy to cache
			proxyByPID[p.Pid] = proxy
			proxyByTarget[proxy.target] = proxy
		}
	}

	d.proxyByPID = proxyByPID
	d.proxyByTarget = proxyByTarget
}

// Filter all connections that have a docker-proxy at one end
func (d *DockerProxy) Filter(payload *model.Connections) {
	if len(d.proxyByPID) == 0 {
		return
	}

	// Discover proxy IPs
	// TODO: we can probably discard the whole logic below if we determine that each proxy
	// instance will be always bound to the docker0 IP
	for _, c := range payload.Conns {
		if len(d.proxyByPID) == 0 {
			break
		}

		if proxy, ok := d.proxyByPID[c.Pid]; ok && proxy != nil {
			if proxyIP := d.discoverProxyIP(proxy, c); proxyIP != "" {
				proxy.ip = proxyIP
				delete(d.proxyByPID, c.Pid)
			}
		}
	}

	// Filter out proxy traffic
	filtered := make([]*model.Connection, 0, len(payload.Conns))
	for _, c := range payload.Conns {
		// If either end of the connection is a proxy we can drop it
		if d.isProxied(c) {
			continue
		}

		filtered = append(filtered, c)
	}

	payload.Conns = filtered
}

func (d *DockerProxy) isProxied(c *model.Connection) bool {
	if p, ok := d.proxyByTarget[model.ContainerAddr{Ip: c.Laddr.Ip, Port: c.Laddr.Port, Protocol: c.Type}]; ok {
		return p.ip == c.Raddr.Ip
	}

	if p, ok := d.proxyByTarget[model.ContainerAddr{Ip: c.Raddr.Ip, Port: c.Raddr.Port, Protocol: c.Type}]; ok {
		return p.ip == c.Laddr.Ip
	}

	return false
}

func (d *DockerProxy) discoverProxyIP(p *proxy, c *model.Connection) string {
	// The heuristic here goes as follows:
	// One of the ends of this connections must match p.target;
	// The proxy IP will be the other end;
	if c.Laddr.Ip == p.target.Ip && c.Laddr.Port == p.target.Port {
		return c.Raddr.Ip
	}

	if c.Raddr.Ip == p.target.Ip && c.Raddr.Port == p.target.Port {
		return c.Laddr.Ip
	}

	return ""
}

func extractProxyTarget(p *procutil.Process) *proxy {
	if len(p.Cmdline) == 0 {
		return nil
	}

	// Sometimes we get all arguments in the first element of the slice
	cmd := p.Cmdline
	if len(cmd) == 1 {
		cmd = strings.Split(cmd[0], " ")
	}

	if !strings.HasSuffix(cmd[0], "docker-proxy") {
		return nil
	}

	// Extract proxy target address
	proxy := &proxy{pid: p.Pid}
	for i := 0; i < len(cmd)-1; i++ {
		switch cmd[i] {
		case "-container-ip":
			proxy.target.Ip = cmd[i+1]
		case "-container-port":
			port, err := strconv.ParseInt(cmd[i+1], 10, 32)
			if err != nil {
				return nil
			}
			proxy.target.Port = int32(port)
		case "-proto":
			name := cmd[i+1]
			proto, ok := model.ConnectionType_value[name]
			if !ok {
				return nil
			}
			proxy.target.Protocol = model.ConnectionType(proto)
		}
	}

	if proxy.target.Ip == "" || proxy.target.Port == 0 {
		return nil
	}

	return proxy
}
