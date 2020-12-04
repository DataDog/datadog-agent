// +build linux
// +build !android

package netlink

import (
	"encoding/binary"

	ct "github.com/florianl/go-conntrack"
	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

// EncodeConn netlink encodes a `Con` object
func EncodeConn(conn *Con) ([]byte, error) {
	ae := netlink.NewAttributeEncoder()
	var err error
	if conn.Con.Origin != nil {
		ae.Nested(ctaTupleOrig, func(nae *netlink.AttributeEncoder) error {
			err = marshalIPTuple(nae, conn.Con.Origin)
			return err
		})

		if err != nil {
			return nil, err
		}
	}

	if conn.Con.Reply != nil {
		ae.Nested(ctaTupleReply, func(nae *netlink.AttributeEncoder) error {
			err = marshalIPTuple(nae, conn.Con.Reply)
			return err
		})

		if err != nil {
			return nil, err
		}
	}

	return ae.Encode()
}

func marshalIPTuple(ae *netlink.AttributeEncoder, tuple *ct.IPTuple) error {
	var err error
	ae.Nested(ctaTupleIP, func(nae *netlink.AttributeEncoder) error {
		if tuple.Src != nil {
			i4 := tuple.Src.To4()
			if i4 != nil {
				nae.Bytes(ctaIPv4Src, i4)
			} else {
				nae.Bytes(ctaIPv6Src, *tuple.Src)
			}
		}

		if tuple.Dst != nil {
			i4 := tuple.Dst.To4()
			if i4 != nil {
				nae.Bytes(ctaIPv4Dst, i4)
			} else {
				nae.Bytes(ctaIPv6Dst, *tuple.Dst)
			}
		}

		return nil
	})

	if tuple.Proto != nil {
		ae.Nested(ctaTupleProto, func(nae *netlink.AttributeEncoder) error {
			err = marshalProto(nae, tuple.Proto)
			return err
		})

		if err != nil {
			return err
		}
	}

	return err
}

func marshalProto(ae *netlink.AttributeEncoder, proto *ct.ProtoTuple) error {
	ae.ByteOrder = binary.BigEndian
	ae.Uint8(ctaProtoNum, *proto.Number)
	ae.Uint16(ctaProtoSrcPort, *proto.SrcPort)
	ae.Uint16(ctaProtoDstPort, *proto.DstPort)
	ae.ByteOrder = nlenc.NativeEndian()
	return nil
}
