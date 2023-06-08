package traps

import (
	"github.com/gosnmp/gosnmp"
	"net"
)

// SnmpPacket is the type of packets yielded by server listeners.
type SnmpPacket struct {
	Content   *gosnmp.SnmpPacket
	Addr      *net.UDPAddr
	Namespace string
	Timestamp int64
}

// PacketsChannel is the type of channels of trap packets.
type PacketsChannel = chan *SnmpPacket

// GetTags returns a list of tags associated to an SNMP trap packet.
func (p *SnmpPacket) getTags() []string {
	return []string{
		"snmp_version:" + formatVersion(p.Content),
		"device_namespace:" + p.Namespace,
		"snmp_device:" + p.Addr.IP.String(),
	}
}
