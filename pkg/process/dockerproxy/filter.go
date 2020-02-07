package dockerproxy

import (
	"strconv"
	"strings"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/gopsutil/process"
)

// Filter keeps track of every docker-proxy instance and filters network traffic going through them
type Filter struct {
	proxyByTarget map[model.ContainerAddr]*proxy
	// This "secondary index" is used only during the proxy IP discovery process
	proxyByPID map[int32]*proxy
}

type proxy struct {
	pid    int32
	ip     string
	target model.ContainerAddr
}

// NewFilter instantiates a new filter loaded with docker-proxy instance information
func NewFilter() *Filter {
	filter := new(Filter)
	if procs, err := process.AllProcesses(); err == nil {
		filter.LoadProxies(procs)
	} else {
		log.Errorf("error initiating proxy filter: %s", err)
	}

	return filter
}

// LoadProxies by inspecting processes information
func (f *Filter) LoadProxies(procs map[int32]*process.FilledProcess) {
	f.proxyByPID = make(map[int32]*proxy)
	f.proxyByTarget = make(map[model.ContainerAddr]*proxy)

	for _, p := range procs {
		proxy := extractProxyTarget(p)
		if proxy == nil {
			continue
		}

		log.Debugf("detected docker-proxy with pid=%d target.ip=%s target.port=%d target.proto=%s",
			proxy.pid,
			proxy.target.Ip,
			proxy.target.Port,
			proxy.target.Protocol,
		)

		// Add proxy to cache
		f.proxyByPID[proxy.pid] = proxy
		f.proxyByTarget[proxy.target] = proxy
	}
}

// Filter all connections that have a docker-proxy at one end
func (f *Filter) Filter(payload *model.Connections) {
	if len(f.proxyByPID) == 0 {
		return
	}

	// Discover proxy IPs
	// TODO: we can probably discard the whole logic below if we determine that each proxy
	// instance will be always bound to the docker0 IP
	for _, c := range payload.Conns {
		if len(f.proxyByPID) == 0 {
			break
		}

		if proxy, ok := f.proxyByPID[c.Pid]; ok {
			if proxyIP := f.discoverProxyIP(proxy, c); proxyIP != "" {
				proxy.ip = proxyIP
				delete(f.proxyByPID, c.Pid)
			}
		}
	}

	// Filter out proxy traffic
	filtered := make([]*model.Connection, 0, len(payload.Conns))
	for _, c := range payload.Conns {
		// If either end of the connection is a proxy we can drop it
		if f.isProxied(c) {
			continue
		}

		filtered = append(filtered, c)
	}

	payload.Conns = filtered
}

func (f *Filter) isProxied(c *model.Connection) bool {
	if p, ok := f.proxyByTarget[model.ContainerAddr{Ip: c.Laddr.Ip, Port: c.Laddr.Port, Protocol: c.Type}]; ok {
		return p.ip == c.Raddr.Ip
	}

	if p, ok := f.proxyByTarget[model.ContainerAddr{Ip: c.Raddr.Ip, Port: c.Raddr.Port, Protocol: c.Type}]; ok {
		return p.ip == c.Laddr.Ip
	}

	return false
}

func (f *Filter) discoverProxyIP(p *proxy, c *model.Connection) string {
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

func extractProxyTarget(p *process.FilledProcess) *proxy {
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
			port, err := strconv.Atoi(cmd[i+1])
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
