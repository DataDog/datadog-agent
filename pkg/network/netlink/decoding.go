// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"encoding/binary"
	"fmt"
	"net/netip"

	"github.com/DataDog/datadog-agent/pkg/util/log"
)

const (
	_ = iota
	ctaTupleOrig
	ctaTupleReply
)

const (
	ctaTupleIP    = 1
	ctaTupleProto = 2
	ctaTupleZone  = 3 //nolint:deadcode
)

const (
	ctaIPv4Src = 1
	ctaIPv4Dst = 2
	ctaIPv6Src = 3
	ctaIPv6Dst = 4
)

const (
	ctaProtoNum     = 1
	ctaProtoSrcPort = 2
	ctaProtoDstPort = 3
)

// ConTuple represents a tuple within a conntrack entry
type ConTuple struct {
	Src   netip.AddrPort
	Dst   netip.AddrPort
	Proto uint8
}

// IsZero returns c is its zero value
func (c ConTuple) IsZero() bool {
	return AddrPortIsZero(c.Src) && AddrPortIsZero(c.Dst) && c.Proto == 0
}

// Con represents a conntrack entry, along with any network namespace info (nsid)
type Con struct {
	Origin ConTuple
	Reply  ConTuple
	NetNS  uint32
}

func (c Con) String() string {
	return fmt.Sprintf("netns=%d src=%s dst=%s sport=%d dport=%d src=%s dst=%s sport=%d dport=%d proto=%d", c.NetNS, c.Origin.Src.Addr(), c.Origin.Dst.Addr(), c.Origin.Src.Port(), c.Origin.Dst.Port(), c.Reply.Src.Addr(), c.Reply.Dst.Addr(), c.Reply.Src.Port(), c.Reply.Dst.Port(), c.Origin.Proto)
}

// Decoder is responsible for decoding netlink messages
type Decoder struct {
	scanner *AttributeScanner
}

// NewDecoder returns a new netlink message Decoder
func NewDecoder() *Decoder {
	return &Decoder{
		scanner: NewAttributeScanner(),
	}
}

// DecodeAndReleaseEvent decodes a single Event into a slice of []ct.Con objects and
// releases the underlying buffer.
func (d *Decoder) DecodeAndReleaseEvent(e Event) []Con {
	msgs := e.Messages()
	conns := make([]Con, 0, len(msgs))

	for _, msg := range msgs {
		c := &Con{NetNS: e.netns}
		if err := d.scanner.ResetTo(msg.Data); err != nil {
			log.Debugf("error decoding netlink message: %s", err)
			continue
		}
		err := d.unmarshalCon(c)
		if err != nil {
			log.Debugf("error decoding netlink message: %s", err)
			continue
		}
		conns = append(conns, *c)
	}

	// Return buffers to the pool
	e.Done()

	return conns
}

func (d *Decoder) unmarshalCon(c *Con) error {
	for toDecode := 2; toDecode > 0 && d.scanner.Next(); {
		switch d.scanner.Type() {
		case ctaTupleOrig:
			toDecode--
			d.scanner.Nested(func() error {
				return d.unmarshalTuple(&c.Origin)
			})
		case ctaTupleReply:
			toDecode--
			d.scanner.Nested(func() error {
				return d.unmarshalTuple(&c.Reply)
			})
		}
	}

	return d.scanner.Err()
}

func (d *Decoder) unmarshalTuple(t *ConTuple) error {
	for toDecode := 2; toDecode > 0 && d.scanner.Next(); {
		switch d.scanner.Type() {
		case ctaTupleIP:
			toDecode--
			d.scanner.Nested(func() error {
				return d.unmarshalTupleIP(t)
			})
		case ctaTupleProto:
			toDecode--
			d.scanner.Nested(func() error {
				return d.unmarshalProto(t)
			})
		}
	}
	return d.scanner.Err()
}

// We might also want to consider deferring the allocation of the IP byte slice
func (d *Decoder) unmarshalTupleIP(t *ConTuple) error {
	for toDecode := 2; toDecode > 0 && d.scanner.Next(); {
		switch d.scanner.Type() {
		case ctaIPv4Src:
			toDecode--
			ip, err := ipv4(d.scanner.Bytes())
			if err != nil {
				return err
			}
			t.Src = AddrPortWithAddr(t.Src, ip)
		case ctaIPv6Src:
			toDecode--
			ip, err := ipv6(d.scanner.Bytes())
			if err != nil {
				return err
			}
			t.Src = AddrPortWithAddr(t.Src, ip)
		case ctaIPv4Dst:
			toDecode--
			ip, err := ipv4(d.scanner.Bytes())
			if err != nil {
				return err
			}
			t.Dst = AddrPortWithAddr(t.Dst, ip)
		case ctaIPv6Dst:
			toDecode--
			ip, err := ipv6(d.scanner.Bytes())
			if err != nil {
				return err
			}
			t.Dst = AddrPortWithAddr(t.Dst, ip)
		}
	}

	return d.scanner.Err()
}

func (d *Decoder) unmarshalProto(t *ConTuple) error {
	for toDecode := 3; toDecode > 0 && d.scanner.Next(); {
		switch d.scanner.Type() {
		case ctaProtoNum:
			toDecode--
			protoNum := d.scanner.Bytes()[0]
			t.Proto = protoNum
		case ctaProtoSrcPort:
			toDecode--
			port := binary.BigEndian.Uint16(d.scanner.Bytes())
			t.Src = AddrPortWithPort(t.Src, port)
		case ctaProtoDstPort:
			toDecode--
			port := binary.BigEndian.Uint16(d.scanner.Bytes())
			t.Dst = AddrPortWithPort(t.Dst, port)
		}
	}

	return d.scanner.Err()
}

func ipv4(b []byte) (netip.Addr, error) {
	if len(b) != 4 {
		return netip.Addr{}, fmt.Errorf("invalid IPv4 size")
	}
	return netip.AddrFrom4(*(*[4]byte)(b)), nil
}

func ipv6(b []byte) (netip.Addr, error) {
	if len(b) != 16 {
		return netip.Addr{}, fmt.Errorf("invalid IPv6 size")
	}
	return netip.AddrFrom16(*(*[16]byte)(b)), nil
}
