// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package npm

import (
	"net"
	"testing"

	agentmodel "github.com/DataDog/agent-payload/v5/process"
	krpretty "github.com/kr/pretty"

	"github.com/stretchr/testify/require"
)

func validateAddr(t *testing.T, addr *agentmodel.Addr) {
	require.NotEqual(t, 0, len(addr.Ip), "addr.Ip = 0")
	require.NotEqual(t, 0, addr.Port, "addr.Port = 0")
	require.Lessf(t, addr.Port, 65536, "addr.Port > 16bits %d", addr.Port)

	require.NotNilf(t, net.ParseIP(addr.Ip), "IP address not valid %s", addr.Ip)
}

func validateConnection(t *testing.T, c *agentmodel.Connection) {
	require.NotZero(t, c.Pid, "Pid = 0")
	require.NotZero(t, c.NetNS, "network namespace = 0")
	require.NotNil(t, c.Laddr, "Laddr is nil")
	require.NotNil(t, c.Raddr, "Raddr is nil")

	validateAddr(t, c.Laddr)
	validateAddr(t, c.Raddr)

	// un-comment the line below when https://datadoghq.atlassian.net/browse/NPM-2958 will be fixed
	// require.False(t, c.LastPacketsSent == 0 && c.LastPacketsReceived == 0, "connection with no packets")

	switch c.Type {
	case agentmodel.ConnectionType_tcp:
		validateTCPConnection(t, c)
	case agentmodel.ConnectionType_udp:
		validateUDPConnection(t, c)
	}

	validateDNSConnection(t, c)
}

func validateDNSConnection(t *testing.T, c *agentmodel.Connection) {
	if c.DnsFailedResponses > 0 || c.DnsSuccessfulResponses > 0 {
		t.Logf(krpretty.Sprintf("DNS %# v", c))
	}
}

func validateTCPConnection(t *testing.T, c *agentmodel.Connection) {
	require.Equal(t, c.Type, agentmodel.ConnectionType_tcp, "connection is not TCP")
	/*
			The following fields depend on the connection state/scenario
			 * short term connection
			 * long term connection
			 * TCP retransmit packets

		  	   require.NotZero(t, c.LastRetransmits, "LastRetransmits = 0")
			   require.NotZero(t, c.LastTcpClosed, "LastTcpClosed = 0")
			   require.NotZero(t, c.LastTcpEstablished, "LastTcpEstablished = 0")
	*/

	// un-comment the lines below when https://datadoghq.atlassian.net/browse/NPM-2958 will be fixed
	// require.NotZero(t, c.Rtt, "Rtt = 0")
	// require.NotZero(t, c.RttVar, "RttVar = 0")
}

func validateUDPConnection(t *testing.T, c *agentmodel.Connection) {
	require.Equal(t, c.Type, agentmodel.ConnectionType_udp, "connection is not UDP")

	require.Zero(t, c.Rtt, "Rtt != 0")
	require.Zero(t, c.RttVar, "RttVar != 0")

	// we can this only for UDP connection as there are no empty payload packets
	// technically possible but in reality no UDP protocol implement that
	// require.False(t, c.LastBytesSent == 0 && c.LastBytesReceived == 0, "connection with no packet bytes")
}

func printDNS(t *testing.T, c *agentmodel.Connection, cc *agentmodel.CollectorConnections, hostname string) {
	if len(c.DnsStatsByDomainOffsetByQueryType) == 0 {
		return
	}
	for domain, stats := range c.DnsStatsByDomainOffsetByQueryType {
		for queryType, dnsstat := range stats.DnsStatsByQueryType {
			domainName, err := cc.GetDNSNameByOffset(domain)
			require.NoErrorf(t, err, "can't resolve domain tags on %s connection %s", hostname, krpretty.Sprint(c))
			t.Logf("DNS %s query type %v %s", domainName, queryType, krpretty.Sprint(dnsstat))
			t.Logf("connection to %s:%d", c.Raddr.Ip, c.Raddr.Port)
		}
	}
}
