// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"net"
	"strings"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	krpretty "github.com/kr/pretty"
	"github.com/stretchr/testify/assert"
)

var helperCurrentHostname string
var helperCurrentConnection *agentmodel.Connection

func helperCleanup(t *testing.T) {
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf(krpretty.Sprintf("test failed on host %s at connection %# v", helperCurrentHostname, helperCurrentConnection))
		}
	})
}

func isWindows(cc *agentmodel.CollectorConnections) bool {
	return strings.Contains(cc.Platform, "Windows")
}

func validateAddr(t *testing.T, addr *agentmodel.Addr) {
	assert.NotEmpty(t, addr.Ip, "addr.Ip = 0")
	assert.GreaterOrEqualf(t, addr.Port, int32(1), "addr.Port < 1 %d", addr.Port)
	assert.Lessf(t, addr.Port, int32(65536), "addr.Port > 16bits %d", addr.Port)

	assert.NotNilf(t, net.ParseIP(addr.Ip), "IP address not valid %s", addr.Ip)
}

func validateConnection(t *testing.T, c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
	helperCurrentHostname = hostname
	helperCurrentConnection = c

	assert.NotZero(t, c.Pid, "Pid = 0")

	if isWindows(cc) {
		assert.Zero(t, c.NetNS, "network namespace != 0")
	} else {
		assert.NotZero(t, c.NetNS, "network namespace = 0")
	}
	assert.NotNil(t, c.Laddr, "Laddr is nil")
	assert.NotNil(t, c.Raddr, "Raddr is nil")

	validateAddr(t, c.Laddr)
	validateAddr(t, c.Raddr)

	// un-comment the line below when https://datadoghq.atlassian.net/browse/NPM-2958 will be fixed
	// assert.False(t, c.LastPacketsSent == 0 && c.LastPacketsReceived == 0, "connection with no packets")

	switch c.Type {
	case agentmodel.ConnectionType_tcp:
		validateTCPConnection(t, c)
	case agentmodel.ConnectionType_udp:
		validateUDPConnection(t, c)
	}

	validateDNSConnection(t, c, cc, hostname)
}

func validateDNSConnection(t *testing.T, c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
	if len(c.DnsStatsByDomainOffsetByQueryType) == 0 {
		return
	}
	for domain, stats := range c.DnsStatsByDomainOffsetByQueryType {
		for queryType, dnsstat := range stats.DnsStatsByQueryType {
			domainName, err := cc.GetDNSNameByOffset(domain)
			assert.NoErrorf(t, err, "can't resolve domain tags on %s connection %s", hostname, krpretty.Sprint(c))
			assert.Greaterf(t, len(domainName), 0, "len(domainName) = 0 %s connection %s", hostname, krpretty.Sprint(c))
			t.Logf("DNS %s query type %v %s", domainName, queryType, krpretty.Sprint(dnsstat))
			t.Logf("connection to %s:%d", c.Raddr.Ip, c.Raddr.Port)
		}
	}
}

func validateTCPConnection(t *testing.T, c *agentmodel.Connection) {
	assert.Equal(t, c.Type, agentmodel.ConnectionType_tcp, "connection is not TCP")

	// un-comment the lines below when https://datadoghq.atlassian.net/browse/NPM-2958 will be fixed
	// assert.NotZero(t, c.Rtt, "Rtt = 0")
	// assert.NotZero(t, c.RttVar, "RttVar = 0")
	//
	// assert.NotZero(t, c.LastRetransmits, "LastRetransmits = 0")
	// assert.NotZero(t, c.LastTcpClosed, "LastTcpClosed = 0")
	// assert.NotZero(t, c.LastTcpEstablished, "LastTcpEstablished = 0")
}

func validateUDPConnection(t *testing.T, c *agentmodel.Connection) {
	assert.Equal(t, c.Type, agentmodel.ConnectionType_udp, "connection is not UDP")

	assert.Zero(t, c.Rtt, "Rtt != 0")
	assert.Zero(t, c.RttVar, "RttVar != 0")
	assert.Zero(t, c.LastRetransmits, "LastRetransmits != 0")
	assert.Zero(t, c.LastTcpEstablished, "LastTcpEstablished != 0")
	assert.Zero(t, c.LastTcpClosed, "LastTcpClosed != 0")

	// we can this only for UDP connection as there are no empty payload packets
	// technically possible but in reality no UDP protocol implement that
	// assert.False(t, c.LastBytesSent == 0 && c.LastBytesReceived == 0, "connection with no packet bytes")
}
