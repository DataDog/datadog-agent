package producer

import (
	"encoding/binary"
	"github.com/DataDog/datadog-agent/comp/netflow/common"
	"github.com/netsampler/goflow2/v2/producer/proto"
	"net"
)

// ParseEthernetHeader parses ethernet header and populate a common.Flow
func ParseEthernetHeader(flow *common.Flow, data []byte, config *HeaderMapper) {
	var countMpls uint32

	var nextHeader byte
	var tcpflags byte
	srcIP := net.IP{}
	dstIP := net.IP{}
	offset := 14

	var srcMac uint64
	var dstMac uint64

	var tos byte
	var fragOffset uint16

	var srcPort uint16
	var dstPort uint16

	for _, configLayer := range GetSFlowConfigLayer(config, "0") {
		extracted := protoproducer.GetBytes(data, configLayer.Offset, configLayer.Length)
		MapCustom(flow, extracted, configLayer.MapConfigBase)
	}

	etherType := data[12:14]

	dstMac = binary.BigEndian.Uint64(append([]byte{0, 0}, data[0:6]...))
	srcMac = binary.BigEndian.Uint64(append([]byte{0, 0}, data[6:12]...))
	(*flow).SrcMac = srcMac
	(*flow).DstMac = dstMac

	encap := true
	iterations := 0
	for encap && iterations <= 1 {
		encap = false

		if etherType[0] == 0x81 && etherType[1] == 0x0 { // VLAN 802.1Q
			offset += 4
			etherType = data[16:18]
		}

		if etherType[0] == 0x88 && etherType[1] == 0x47 { // MPLS
			iterateMpls := true
			for iterateMpls {
				if len(data) < offset+5 {
					iterateMpls = false
					break
				}
				label := binary.BigEndian.Uint32(append([]byte{0}, data[offset:offset+3]...)) >> 4
				bottom := data[offset+2] & 1
				offset += 4

				if bottom == 1 || label <= 15 || offset > len(data) {
					if data[offset]&0xf0>>4 == 4 {
						etherType = []byte{0x8, 0x0}
					} else if data[offset]&0xf0>>4 == 6 {
						etherType = []byte{0x86, 0xdd}
					}
					iterateMpls = false
				}

				countMpls++
			}
		}

		for _, configLayer := range GetSFlowConfigLayer(config, "3") {
			extracted := protoproducer.GetBytes(data, offset*8+configLayer.Offset, configLayer.Length)
			MapCustom(flow, extracted, configLayer.MapConfigBase)
		}

		if etherType[0] == 0x8 && etherType[1] == 0x0 { // IPv4
			if len(data) >= offset+20 {
				nextHeader = data[offset+9]
				srcIP = data[offset+12 : offset+16]
				dstIP = data[offset+16 : offset+20]
				tos = data[offset+1]

				fragOffset = binary.BigEndian.Uint16(data[offset+6:offset+8]) & 8191

				offset += 20
			}
		} else if etherType[0] == 0x86 && etherType[1] == 0xdd { // IPv6
			if len(data) >= offset+40 {
				nextHeader = data[offset+6]
				srcIP = data[offset+8 : offset+24]
				dstIP = data[offset+24 : offset+40]

				tostmp := uint32(binary.BigEndian.Uint16(data[offset : offset+2]))
				tos = uint8(tostmp & 0x0ff0 >> 4)

				offset += 40

			}
		}

		for _, configLayer := range GetSFlowConfigLayer(config, "4") {
			extracted := protoproducer.GetBytes(data, offset*8+configLayer.Offset, configLayer.Length)
			MapCustom(flow, extracted, configLayer.MapConfigBase)
		}

		appOffset := 0
		if len(data) >= offset+4 && (nextHeader == 17 || nextHeader == 6) && fragOffset&8191 == 0 {
			srcPort = binary.BigEndian.Uint16(data[offset+0 : offset+2])
			dstPort = binary.BigEndian.Uint16(data[offset+2 : offset+4])
		}

		if nextHeader == 17 {
			appOffset = 8
		}

		if len(data) > offset+13 && nextHeader == 6 {
			tcpflags = data[offset+13]

			appOffset = int(data[13]>>4) * 4
		}

		if appOffset > 0 {
			for _, configLayer := range GetSFlowConfigLayer(config, "7") {
				extracted := protoproducer.GetBytes(data, (offset+appOffset)*8+configLayer.Offset, configLayer.Length)
				MapCustom(flow, extracted, configLayer.MapConfigBase)
			}
		}

		iterations++
	}

	// TODO : move this up to avoid overiding custom values
	(*flow).EtherType = uint32(binary.BigEndian.Uint16(etherType[0:2]))

	(*flow).SrcPort = int32(srcPort)
	(*flow).DstPort = int32(dstPort)

	(*flow).SrcAddr = srcIP
	(*flow).DstAddr = dstIP
	(*flow).IPProtocol = uint32(nextHeader)
	(*flow).Tos = uint32(tos)
	(*flow).TCPFlags = uint32(tcpflags)
}
