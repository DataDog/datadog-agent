// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"encoding/binary"

	"github.com/mdlayher/netlink"

	"github.com/DataDog/datadog-agent/pkg/util/native"
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
		if !AddrIsZero(tuple.Src.Addr()) {
			if tuple.Src.Addr().Is4() || tuple.Src.Addr().Is4In6() {
				b := tuple.Src.Addr().As4()
				nae.Bytes(ctaIPv4Src, b[:])
			} else {
				b := tuple.Src.Addr().As16()
				nae.Bytes(ctaIPv6Src, b[:])
			}
		}

		if !AddrIsZero(tuple.Dst.Addr()) {
			if tuple.Dst.Addr().Is4() || tuple.Dst.Addr().Is4In6() {
				b := tuple.Dst.Addr().As4()
				nae.Bytes(ctaIPv4Dst, b[:])
			} else {
				b := tuple.Dst.Addr().As16()
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
	ae.ByteOrder = native.Endian
	return nil
}
