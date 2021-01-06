package encoding

import (
	"sync"

	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

var connsPool = sync.Pool{
	New: func() interface{} {
		return new(model.Connections)
	},
}

var connPool = sync.Pool{
	New: func() interface{} {
		return new(model.Connection)
	},
}

// FormatConnection converts a ConnectionStats into an model.Connection
func FormatConnection(conn network.ConnectionStats, domainSet map[string]int) *model.Connection {
	c := connPool.Get().(*model.Connection)
	c.Pid = int32(conn.Pid)
	c.Laddr = formatAddr(conn.Source, conn.SPort)
	c.Raddr = formatAddr(conn.Dest, conn.DPort)
	c.Family = formatFamily(conn.Family)
	c.Type = formatType(conn.Type)
	c.PidCreateTime = 0
	c.LastBytesSent = conn.LastSentBytes
	c.LastBytesReceived = conn.LastRecvBytes
	c.LastRetransmits = conn.LastRetransmits
	c.Direction = formatDirection(conn.Direction)
	c.NetNS = conn.NetNS
	c.RemoteNetworkId = ""
	c.IpTranslation = formatIPTranslation(conn.IPTranslation)
	c.Rtt = conn.RTT
	c.RttVar = conn.RTTVar
	c.IntraHost = conn.IntraHost
	c.DnsSuccessfulResponses = conn.DNSSuccessfulResponses
	c.DnsFailedResponses = conn.DNSFailedResponses
	c.DnsTimeouts = conn.DNSTimeouts
	c.DnsSuccessLatencySum = conn.DNSSuccessLatencySum
	c.DnsFailureLatencySum = conn.DNSFailureLatencySum
	c.DnsCountByRcode = conn.DNSCountByRcode
	c.LastTcpEstablished = conn.LastTCPEstablished
	c.LastTcpClosed = conn.LastTCPClosed
	c.DnsStatsByDomain = formatDNSStatsByDomain(conn.DNSStatsByDomain, domainSet)
	return c
}

var dnsPool = sync.Pool{
	New: func() interface{} {
		return new(model.DNSEntry)
	},
}

// FormatDNS converts a map[util.Address][]string to a map using IPs string representation
func FormatDNS(dns map[util.Address][]string) map[string]*model.DNSEntry {
	if dns == nil {
		return nil
	}

	ipToNames := make(map[string]*model.DNSEntry, len(dns))
	for addr, names := range dns {
		entry := dnsPool.Get().(*model.DNSEntry)
		entry.Names = names
		ipToNames[addr.String()] = entry
	}

	return ipToNames
}

var telemetryPool = sync.Pool{
	New: func() interface{} {
		return new(model.ConnectionsTelemetry)
	},
}

// FormatTelemetry converts telemetry from its internal representation to a protobuf message
func FormatTelemetry(tel *network.ConnectionsTelemetry) *model.ConnectionsTelemetry {
	if tel == nil {
		return nil
	}

	t := telemetryPool.Get().(*model.ConnectionsTelemetry)
	t.MonotonicKprobesTriggered = tel.MonotonicKprobesTriggered
	t.MonotonicKprobesMissed = tel.MonotonicKprobesMissed
	t.MonotonicConntrackRegisters = tel.MonotonicConntrackRegisters
	t.MonotonicConntrackRegistersDropped = tel.MonotonicConntrackRegistersDropped
	t.MonotonicDnsPacketsProcessed = tel.MonotonicDNSPacketsProcessed
	t.MonotonicConnsClosed = tel.MonotonicConnsClosed
	t.ConnsBpfMapSize = tel.ConnsBpfMapSize
	t.MonotonicUdpSendsProcessed = tel.MonotonicUDPSendsProcessed
	t.MonotonicUdpSendsMissed = tel.MonotonicUDPSendsMissed
	t.ConntrackSamplingPercent = tel.ConntrackSamplingPercent
	return t
}

func returnToPool(c *model.Connections) {
	if c.Conns != nil {
		for _, c := range c.Conns {
			connPool.Put(c)
		}
	}
	if c.Dns != nil {
		for _, e := range c.Dns {
			dnsPool.Put(e)
		}
	}
	telemetryPool.Put(c.Telemetry)
	connsPool.Put(c)
}

func formatAddr(addr util.Address, port uint16) *model.Addr {
	if addr == nil {
		return nil
	}

	return &model.Addr{Ip: addr.String(), Port: int32(port)}
}

func formatFamily(f network.ConnectionFamily) model.ConnectionFamily {
	switch f {
	case network.AFINET:
		return model.ConnectionFamily_v4
	case network.AFINET6:
		return model.ConnectionFamily_v6
	default:
		return -1
	}
}

func formatType(f network.ConnectionType) model.ConnectionType {
	switch f {
	case network.TCP:
		return model.ConnectionType_tcp
	case network.UDP:
		return model.ConnectionType_udp
	default:
		return -1
	}
}

func formatDirection(d network.ConnectionDirection) model.ConnectionDirection {
	switch d {
	case network.INCOMING:
		return model.ConnectionDirection_incoming
	case network.OUTGOING:
		return model.ConnectionDirection_outgoing
	case network.LOCAL:
		return model.ConnectionDirection_local
	case network.NONE:
		return model.ConnectionDirection_none
	default:
		return model.ConnectionDirection_unspecified
	}
}

func formatDNSStatsByDomain(stats map[string]network.DNSStats, domainSet map[string]int) map[int32]*model.DNSStats {
	m := make(map[int32]*model.DNSStats)
	for d, s := range stats {
		var ms model.DNSStats
		ms.DnsCountByRcode = s.DNSCountByRcode
		ms.DnsFailureLatencySum = s.DNSFailureLatencySum
		ms.DnsSuccessLatencySum = s.DNSSuccessLatencySum
		ms.DnsTimeouts = s.DNSTimeouts
		pos, ok := domainSet[d]
		if !ok {
			pos = len(domainSet)
			domainSet[d] = pos
		}
		m[int32(pos)] = &ms
	}
	return m
}

func formatIPTranslation(ct *network.IPTranslation) *model.IPTranslation {
	if ct == nil {
		return nil
	}

	return &model.IPTranslation{
		ReplSrcIP:   ct.ReplSrcIP.String(),
		ReplDstIP:   ct.ReplDstIP.String(),
		ReplSrcPort: int32(ct.ReplSrcPort),
		ReplDstPort: int32(ct.ReplDstPort),
	}
}
