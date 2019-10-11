package encoding

import (
	model "github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// FormatConnection converts a ConnectionStats into an model.Connection
func FormatConnection(conn ebpf.ConnectionStats, useAddrByteString bool) *model.Connection {
	return &model.Connection{
		Pid:               int32(conn.Pid),
		Laddr:             formatAddr(conn.Source, conn.SPort, useAddrByteString),
		Raddr:             formatAddr(conn.Dest, conn.DPort, useAddrByteString),
		Family:            formatFamily(conn.Family),
		Type:              formatType(conn.Type),
		LastBytesSent:     conn.LastSentBytes,
		LastBytesReceived: conn.LastRecvBytes,
		LastRetransmits:   conn.LastRetransmits,
		Rtt:               conn.RTT,
		RttVar:            conn.RTTVar,
		Direction:         formatDirection(conn.Direction),
		NetNS:             conn.NetNS,
		IpTranslation:     formatIPTranslation(conn.IPTranslation, useAddrByteString),
	}
}

// FormatDNS converts a map[util.Address][]string to a map using IPs byte string representation
func FormatDNS(dns map[util.Address][]string, useAddrByteString bool) map[string]*model.DNSEntry {
	if dns == nil {
		return nil
	}

	ipToNames := make(map[string]*model.DNSEntry, len(dns))
	for addr, names := range dns {
		if useAddrByteString {
			ipToNames[addr.ByteString()] = &model.DNSEntry{Names: names}
		} else {
			ipToNames[addr.String()] = &model.DNSEntry{Names: names}
		}
	}

	return ipToNames
}

func formatAddr(addr util.Address, port uint16, useAddrByteString bool) *model.Addr {
	if addr == nil {
		return nil
	}

	address := &model.Addr{
		Port: int32(port),
	}

	if useAddrByteString {
		address.IpByteString = addr.ByteString()
	} else {
		address.Ip = addr.String()
	}

	return address
}

func formatFamily(f ebpf.ConnectionFamily) model.ConnectionFamily {
	switch f {
	case ebpf.AFINET:
		return model.ConnectionFamily_v4
	case ebpf.AFINET6:
		return model.ConnectionFamily_v6
	default:
		return -1
	}
}

func formatType(f ebpf.ConnectionType) model.ConnectionType {
	switch f {
	case ebpf.TCP:
		return model.ConnectionType_tcp
	case ebpf.UDP:
		return model.ConnectionType_udp
	default:
		return -1
	}
}

func formatDirection(d ebpf.ConnectionDirection) model.ConnectionDirection {
	switch d {
	case ebpf.INCOMING:
		return model.ConnectionDirection_incoming
	case ebpf.OUTGOING:
		return model.ConnectionDirection_outgoing
	case ebpf.LOCAL:
		return model.ConnectionDirection_local
	default:
		return model.ConnectionDirection_unspecified
	}
}

func formatIPTranslation(ct *netlink.IPTranslation, useAddrByteString bool) *model.IPTranslation {
	if ct == nil {
		return nil
	}

	t := &model.IPTranslation{
		ReplSrcPort: int32(ct.ReplSrcPort),
		ReplDstPort: int32(ct.ReplDstPort),
	}

	if useAddrByteString {
		t.ReplSrcIPByteString = ct.ReplSrcIP.ByteString()
		t.ReplDstIPByteString = ct.ReplDstIP.ByteString()
	} else {
		t.ReplSrcIP = ct.ReplSrcIP.String()
		t.ReplDstIP = ct.ReplDstIP.String()
	}
	return t
}
