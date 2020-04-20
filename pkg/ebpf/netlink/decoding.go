package netlink

import (
	"bytes"
	"encoding/binary"
	"log"
	"net"

	ct "github.com/florianl/go-conntrack"
	"github.com/mdlayher/netlink"
	"golang.org/x/sys/unix"
)

const (
	ctaUnspec = iota
	ctaTupleOrig
	ctaTupleReply
	ctaStatus
	ctaProtoinfo
	ctaHelp
	ctaNatSrc
	ctaTimeout
	ctaMark
	ctaCountersOrig
	ctaCountersReply
	ctaUse
	ctaID
	ctaNatDst
	ctaTupleMaster
	ctaSeqAdjOrig
	ctaSeqAdjRepl
	ctaSecmark
	ctaZone
	ctaSecCtx
	ctaTimestamp
	ctaMarkMask
	ctaLables
	ctaLablesMask
)

const (
	ctaTupleIP    = 1
	ctaTupleProto = 2
	ctaTupleZone  = 3
)

const (
	ctaIPv4Src = 1
	ctaIPv4Dst = 2
	ctaIPv6Src = 3
	ctaIPv6Dst = 4
)

const (
	ctaProtoNum        = 1
	ctaProtoSrcPort    = 2
	ctaProtoDstPort    = 3
	ctaProtoIcmpID     = 4
	ctaProtoIcmpType   = 5
	ctaProtoIcmpCode   = 6
	ctaProtoIcmpv6ID   = 7
	ctaProtoIcmpv6Type = 8
	ctaProtoIcmpv6Code = 9
)

func DecodeEvent(logger *log.Logger, e Event) ([]ct.Con, error) {
	// Propagate socket error upstream
	if e.Error != nil {
		return nil, e.Error
	}

	conns := make([]ct.Con, 0, len(e.Reply))
	for _, msg := range e.Reply {
		orig, reply, err := decodeMessage(msg)

		if err != nil || !isNATRaw(orig, reply) {
			continue
		}

		c := ct.Con{Origin: orig.IPTuple(), Reply: reply.IPTuple()}
		conns = append(conns, c)
	}

	return conns, nil
}

func decodeMessage(msg netlink.Message) (orig, reply *rawIPTuple, err error) {
	offset := messageOffset(msg.Data[:2])
	ad, err := netlink.NewAttributeDecoder(msg.Data[offset:])
	if err != nil {
		return nil, nil, err
	}

	ad.ByteOrder = binary.BigEndian

	var (
		origTuple  = &rawIPTuple{}
		replyTuple = &rawIPTuple{}
	)

	toDecode := 2

Loop:
	for ad.Next() {
		switch ad.Type() {
		case ctaTupleOrig:
			ad.Nested(origTuple.unmarshall)
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		case ctaTupleReply:
			ad.Nested(replyTuple.unmarshall)
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		}
	}

	return origTuple, replyTuple, ad.Err()
}

func messageOffset(data []byte) int {
	if (data[0] == unix.AF_INET || data[0] == unix.AF_INET6) && data[1] == unix.NFNETLINK_V0 {
		return 4
	}
	return 0
}

func isNATRaw(orig, reply *rawIPTuple) bool {
	if len(orig.srcIP) == 0 || len(orig.dstIP) == 0 || orig.proto == 0 {
		return false
	}

	if len(reply.srcIP) == 0 || len(reply.dstIP) == 0 || reply.proto == 0 {
		return false
	}

	return !bytes.Equal(orig.srcIP, reply.dstIP) ||
		!bytes.Equal(orig.dstIP, reply.srcIP) ||
		orig.srcPort != reply.dstPort ||
		orig.dstPort != reply.srcPort
}

type rawIPTuple struct {
	srcIP   []byte
	dstIP   []byte
	srcPort uint16
	dstPort uint16
	proto   uint8
}

func (r *rawIPTuple) IPTuple() *ct.IPTuple {
	srcIP := net.IP(r.srcIP)
	dstIP := net.IP(r.dstIP)
	return &ct.IPTuple{
		Src: &srcIP,
		Dst: &dstIP,
		Proto: &ct.ProtoTuple{
			Number:  &r.proto,
			SrcPort: &r.srcPort,
			DstPort: &r.dstPort,
		},
	}
}

func (r *rawIPTuple) unmarshall(ad *netlink.AttributeDecoder) error {
	toDecode := 2

Loop:
	for ad.Next() {
		switch ad.Type() {
		case ctaTupleIP:
			ad.Nested(r.unmarshallIP)
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		case ctaTupleProto:
			ad.Nested(r.unmarshallProto)
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		}
	}
	return ad.Err()
}

func (r *rawIPTuple) unmarshallIP(ad *netlink.AttributeDecoder) error {
	toDecode := 2

Loop:
	for ad.Next() {
		switch ad.Type() {
		case ctaIPv4Src, ctaIPv6Src:
			r.srcIP = ad.Bytes()
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		case ctaIPv4Dst, ctaIPv6Dst:
			r.dstIP = ad.Bytes()
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		}
	}

	return ad.Err()
}

func (r *rawIPTuple) unmarshallProto(ad *netlink.AttributeDecoder) error {
	toDecode := 3

Loop:
	for ad.Next() {
		switch ad.Type() {
		case ctaProtoNum:
			r.proto = ad.Uint8()
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		case ctaProtoSrcPort:
			r.srcPort = ad.Uint16()
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		case ctaProtoDstPort:
			r.dstPort = ad.Uint16()
			toDecode--
			if toDecode == 0 {
				break Loop
			}
		}
	}

	return ad.Err()
}
