// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux

package sender

import (
	"net/netip"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	logmock "github.com/DataDog/datadog-agent/comp/core/log/mock"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	"github.com/DataDog/datadog-agent/pkg/security/secl/model"
)

func TestDockerProxyExtract(t *testing.T) {
	// non docker-proxy processes
	require.Nil(t, extractProxyTarget(&process{Pid: 1}))
	require.Nil(t, extractProxyTarget(&process{Pid: 1, Cmdline: []string{"/usr/bin/true"}}))

	cases := []struct {
		ip, port, proto string
		target          containerAddr
	}{
		{},
		{ip: "asdf"},
		{port: "asdf"},
		{proto: "asdf"},
		{ip: "127.0.0.2"},
		{ip: "127.0.0.2", port: "0"},
		{"127.0.0.2", "9999", "tcp", containerAddr{netip.MustParseAddrPort("127.0.0.2:9999"), network.TCP}},
		{"127.0.0.2", "9999", "TCP", containerAddr{netip.MustParseAddrPort("127.0.0.2:9999"), network.TCP}},
		{"127.0.0.2", "9999", "udp", containerAddr{netip.MustParseAddrPort("127.0.0.2:9999"), network.UDP}},
		{"::1", "9999", "udp", containerAddr{netip.MustParseAddrPort("[::1]:9999"), network.UDP}},
	}
	for _, c := range cases {
		cmdline := dockerProxyCmdLine(c.ip, c.port, c.proto)
		p := extractProxyTarget(&process{Pid: 1, Cmdline: cmdline})
		if !c.target.addr.IsValid() {
			require.Nil(t, p, "%+v", c)
		} else {
			require.NotNil(t, p, "%+v", c)
			require.Equal(t, c.target, p.target, "%+v", c)
		}
	}
}

func dockerProxyCmdLine(ip, port, proto string) []string {
	cmdline := []string{"/usr/local/bin/docker-proxy"}
	if ip != "" {
		cmdline = append(cmdline, "-container-ip", ip)
	}
	if port != "" {
		cmdline = append(cmdline, "-container-port", port)
	}
	if proto != "" {
		cmdline = append(cmdline, "-proto", proto)
	}
	return cmdline
}

// The example below represents the following scenario:
//
//   - Two containers (in this example a redis-client and a redis-server) are
//     running on the same host (IP 10.0.2.15)
//
//   - The redis-server binds to host port 32769
//     (`docker run --rm -d -p 6379:32769 redis:alpine`)
//
//   - The redis-client communicates to redis-server via the host IP/port
//     (`docker run --rm redis:alpine redis-cli -h 10.0.2.15 -p 32769 set foo bar`)
//
// Since the two containers are co-located within the same host and the communication
// is done via the Host IP/Port, network traffic flows through docker-proxy.
// This ends up generating the following flows:
//
// 1) redis-client -> redis-server (via host IP)
// 2) redis-server (via host IP) <- redis-client
// 3) docker-proxy -> redis-server (redundant)
// 4) redis-server <- docker-proxy (redundant)
//
// The purpose of this package is to filter flows like (3) and (4) in order to
// avoid double counting traffic represented similar to flows (1) and (2)
func TestDockerProxyFiltering(t *testing.T) {
	dpf := newDockerProxyFilter(logmock.New(t))
	// ensure connections are filtered out
	dockerProxyPID := uint32(23211)
	redisServerPort := uint16(6379)
	redisServerContainerIP := "172.17.0.2"
	redisClientIP := "172.17.0.3"
	redisClientPort := uint16(37340)
	redisServerHostIP := "10.0.2.15"
	redisServerHostPort := uint16(32769)
	dockerProxyIP := "172.17.0.1"
	dockerProxyPort := uint16(34050)
	dpf.process(&process{Pid: dockerProxyPID, Cmdline: dockerProxyCmdLine(redisServerContainerIP, strconv.Itoa(int(redisServerPort)), "tcp"), EventType: model.ExecEventType})

	// (1) This represents the *outgoing* connection from redis client to redis server (via host IP)
	// It should be *kept*
	c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Pid:       24296,
		Source:    util.AddressFromString(redisClientIP),
		SPort:     redisClientPort,
		Dest:      util.AddressFromString(redisServerHostIP),
		DPort:     redisServerHostPort,
		Direction: network.OUTGOING,
	}}

	// (2) This represents the *incoming* connection on redis server from redis client (via host IP)
	// It should be *kept*
	c2 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Pid:       dockerProxyPID,
		Source:    util.AddressFromString(redisServerHostIP),
		SPort:     redisServerHostPort,
		Dest:      util.AddressFromString(redisClientIP),
		DPort:     redisClientPort,
		Direction: network.INCOMING,
	}}

	// (3) This represents the *outgoing* connection from docker-proxy to redis server
	// It should be *dropped*
	c3 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Pid:       dockerProxyPID,
		Source:    util.AddressFromString(dockerProxyIP),
		SPort:     dockerProxyPort,
		Dest:      util.AddressFromString(redisServerContainerIP),
		DPort:     redisServerPort,
		Direction: network.OUTGOING,
	}}

	// (4) This represents the *incoming* connection on redis server from docker proxy
	// It should be *dropped*
	c4 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{
		Pid:       23233,
		Source:    util.AddressFromString(redisServerContainerIP),
		SPort:     redisServerPort,
		Dest:      util.AddressFromString(dockerProxyIP),
		DPort:     dockerProxyPort,
		Direction: network.INCOMING,
	}}

	conns := &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c1, c2, c3, c4}}}
	dpf.FilterProxies(conns)
	require.Len(t, conns.Conns, 2)
	assert.Equal(t, c1, conns.Conns[0])
	assert.Equal(t, c2, conns.Conns[1])
}

func TestDockerProxyEvents(t *testing.T) {
	dpf := newDockerProxyFilter(logmock.New(t))
	dpf.pidAliveFunc = func(_ int) bool { return true }
	// ensure connections are filtered out
	dpf.process(&process{Pid: 10, Cmdline: dockerProxyCmdLine("127.0.0.2", "9999", "tcp"), EventType: model.ExecEventType})
	c0 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 10, Source: util.AddressFromString("127.0.0.2"), SPort: 10000, Dest: util.AddressFromString("127.0.0.2"), DPort: 9999}}
	c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 11, Source: util.AddressFromString("127.0.0.2"), SPort: 9999, Dest: util.AddressFromString("127.0.0.2"), DPort: 10000}}
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Empty(t, conns.Conns)

	// send exit event and ensure next iteration of FilterProxies still filters them out
	dpf.process(&process{Pid: 10, EventType: model.ExitEventType})
	conns = &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Empty(t, conns.Conns)

	// another filter run should not filter anything
	conns = &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Len(t, conns.Conns, 2)
}

func TestDockerProxyPIDOverwrite(t *testing.T) {
	dpf := newDockerProxyFilter(logmock.New(t))
	dpf.pidAliveFunc = func(_ int) bool { return true }
	dpf.process(&process{Pid: 10, Cmdline: dockerProxyCmdLine("127.0.0.2", "9999", "tcp"), EventType: model.ExecEventType})
	c0 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 10, Source: util.AddressFromString("127.0.0.2"), SPort: 10000, Dest: util.AddressFromString("127.0.0.2"), DPort: 9999}}
	c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 11, Source: util.AddressFromString("127.0.0.2"), SPort: 9999, Dest: util.AddressFromString("127.0.0.2"), DPort: 10000}}
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Empty(t, conns.Conns)

	// new process with same PID as previous docker-proxy
	dpf.process(&process{Pid: 10, Cmdline: []string{"/usr/bin/true"}})
	conns = &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Empty(t, conns.Conns)
}

func TestDockerProxyDeadProcess(t *testing.T) {
	dpf := newDockerProxyFilter(logmock.New(t))
	dpf.pidAliveFunc = func(_ int) bool { return false }
	dpf.process(&process{Pid: 10, Cmdline: dockerProxyCmdLine("127.0.0.2", "9999", "tcp"), EventType: model.ExecEventType})
	c0 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 10, Source: util.AddressFromString("127.0.0.2"), SPort: 10000, Dest: util.AddressFromString("127.0.0.2"), DPort: 9999}}
	c1 := network.ConnectionStats{ConnectionTuple: network.ConnectionTuple{Pid: 11, Source: util.AddressFromString("127.0.0.2"), SPort: 9999, Dest: util.AddressFromString("127.0.0.2"), DPort: 10000}}
	conns := &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Empty(t, conns.Conns)

	conns = &network.Connections{BufferedData: network.BufferedData{Conns: []network.ConnectionStats{c0, c1}}}
	dpf.FilterProxies(conns)
	require.Len(t, conns.Conns, 2)
}
