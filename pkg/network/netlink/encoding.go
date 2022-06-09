// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && !android
// +build linux,!android

package netlink

import (
	"encoding/binary"

	"github.com/mdlayher/netlink"
	"github.com/mdlayher/netlink/nlenc"
)

// EncodeConn netlink encodes a `Con` object
func EncodeConn(conn *Con) ([]byte, error) {
	ae := netlink.NewAttributeEncoder()
	var err error
	if !conn.Origin.IsZero() {
		ae.Nested(ctaTupleOrig, func(nae *netlink.AttributeEncoder) error {
			err = marshalIPTuple(nae, &conn.Origin)
			return err
		})

		if err != nil {
			return nil, err
		}
	}

	if !conn.Reply.IsZero() {
		ae.Nested(ctaTupleReply, func(nae *netlink.AttributeEncoder) error {
			err = marshalIPTuple(nae, &conn.Reply)
			return err
		})

		if err != nil {
			return nil, err
		}
	}

	return ae.Encode()
}

func marshalIPTuple(ae *netlink.AttributeEncoder, tuple *ConTuple) error {
	var err error
	ae.Nested(ctaTupleIP, func(nae *netlink.AttributeEncoder) error {
		if !tuple.Src.IP().IsZero() {
			if tuple.Src.IP().Is4() || tuple.Src.IP().Is4in6() {
				b := tuple.Src.IP().As4()
				nae.Bytes(ctaIPv4Src, b[:])
			} else {
				b := tuple.Src.IP().As16()
				nae.Bytes(ctaIPv6Src, b[:])
			}
		}

		if !tuple.Dst.IP().IsZero() {
			if tuple.Dst.IP().Is4() || tuple.Dst.IP().Is4in6() {
				b := tuple.Dst.IP().As4()
				nae.Bytes(ctaIPv4Dst, b[:])
			} else {
				b := tuple.Dst.IP().As16()
				nae.Bytes(ctaIPv6Dst, b[:])
			}
		}

		return nil
	})

	if tuple.Proto != 0 {
		ae.Nested(ctaTupleProto, func(nae *netlink.AttributeEncoder) error {
			err = marshalProto(nae, tuple)
			return err
		})

		if err != nil {
			return err
		}
	}

	return err
}

func marshalProto(ae *netlink.AttributeEncoder, tuple *ConTuple) error {
	ae.ByteOrder = binary.BigEndian
	ae.Uint8(ctaProtoNum, tuple.Proto)
	ae.Uint16(ctaProtoSrcPort, tuple.Src.Port())
	ae.Uint16(ctaProtoDstPort, tuple.Dst.Port())
	ae.ByteOrder = nlenc.NativeEndian()
	return nil
}
