package encoding

import (
	"github.com/DataDog/datadog-agent/pkg/ebpf"
	"github.com/DataDog/datadog-agent/pkg/ebpf/netlink"
	"github.com/DataDog/datadog-agent/pkg/process/model"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

// FormatConnection converts a ConnectionStats into an model.Connection
func FormatConnection(conn ebpf.ConnectionStats) *model.Connection {
	return &model.Connection{
		Pid:                int32(conn.Pid),
		Laddr:              formatAddr(conn.Source, conn.SPort),
		Raddr:              formatAddr(conn.Dest, conn.DPort),
		Family:             formatFamily(conn.Family),
		Type:               formatType(conn.Type),
		TotalBytesSent:     conn.MonotonicSentBytes,
		TotalBytesReceived: conn.MonotonicRecvBytes,
		TotalRetransmits:   conn.MonotonicRetransmits,
		LastBytesSent:      conn.LastSentBytes,
		LastBytesReceived:  conn.LastRecvBytes,
		LastRetransmits:    conn.LastRetransmits,
		Direction:          model.ConnectionDirection(conn.Direction),
		NetNS:              conn.NetNS,
		IpTranslation:      formatIPTranslation(conn.IPTranslation),
	}
}

func formatAddr(addr util.Address, port uint16) *model.Addr {
	if addr == nil {
		return nil
	}

	return &model.Addr{Ip: addr.String(), Port: int32(port)}
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

func formatIPTranslation(ct *netlink.IPTranslation) *model.IPTranslation {
	if ct == nil {
		return nil
	}

	return &model.IPTranslation{
		ReplSrcIP:   ct.ReplSrcIP,
		ReplDstIP:   ct.ReplDstIP,
		ReplSrcPort: int32(ct.ReplSrcPort),
		ReplDstPort: int32(ct.ReplDstPort),
	}
}
