/* SPDX-License-Identifier: BSD-2-Clause */

package net

import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
)

// Version6 is IP version 6
var Version6 = 6

// IPv6HeaderLen is the length of the IPv6 header
var IPv6HeaderLen = 40

// NewIPv6 constructs a new IPv6 header from a sequence of bytes
func NewIPv6(b []byte) (*IPv6, error) {
	var h IPv6
	if err := h.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return &h, nil
}

// IPv6 is the IPv6 header
type IPv6 struct {
	Version      int
	TrafficClass int
	FlowLabel    int
	PayloadLen   int
	NextHeader   IPProto
	HopLimit     int
	Src          net.IP
	Dst          net.IP
	next         Layer
	// IP in ICMP, if set, won't make the parser fail on short packets
	IPinICMP bool
}

// Next returns the next layer
func (h IPv6) Next() Layer {
	return h.next
}

// SetNext sets the next Layer
func (h *IPv6) SetNext(l Layer) {
	h.next = l
}

// MarshalBinary serializes the layer
func (h IPv6) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer
	// Version check
	if h.Version == 0 {
		h.Version = Version6
	}
	if h.Version != Version6 {
		return nil, errors.New("invalid IPv6 version")
	}
	// traffic class
	if h.TrafficClass < 0 || h.TrafficClass > 0xff {
		return nil, errors.New("invalid IPv6 traffic class")
	}
	// flow label
	if h.FlowLabel < 0 || h.FlowLabel > 0x0fffff {
		return nil, errors.New("invalid IPv6 flow label")
	}
	// payload length
	var (
		payload []byte
		err     error
	)
	if h.next != nil {
		payload, err = h.next.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	if h.PayloadLen == 0 {
		h.PayloadLen = len(payload)
	}
	if h.PayloadLen < 0 || h.PayloadLen > 0xffff {
		return nil, errors.New("invalid IPv6 payload length")
	}
	// next header
	if h.NextHeader < 0 || h.NextHeader > 0xff {
		return nil, errors.New("invalid IPv6 next header")
	}
	/*
		if h.NextHeader == 0 {
			switch h.next.(type) {
			case *UDP:
				h.NextHeader = ProtoUDP
			case *ICMPv6:
				h.NextHeader = ProtoICMPv6
			}
		}
	*/
	// hop limit
	if h.HopLimit < 0 || h.HopLimit > 0xff {
		return nil, errors.New("invalid IPv6 hop limit")
	}
	// src and dst
	if h.Src == nil {
		h.Src = net.IPv6zero
	}
	if h.Dst == nil {
		h.Dst = net.IPv6zero
	}

	binary.Write(&b, binary.BigEndian, uint32(h.Version<<28|h.TrafficClass<<20|h.FlowLabel))
	binary.Write(&b, binary.BigEndian, uint16(h.PayloadLen))
	binary.Write(&b, binary.BigEndian, uint8(h.NextHeader))
	binary.Write(&b, binary.BigEndian, uint8(h.HopLimit))
	binary.Write(&b, binary.BigEndian, []byte(h.Src.To16()))
	binary.Write(&b, binary.BigEndian, []byte(h.Dst.To16()))
	ret := b.Bytes()
	// payload
	ret = append(ret, payload...)
	return ret, nil
}

// UnmarshalBinary deserializes the raw bytes to an IPv6 header
func (h *IPv6) UnmarshalBinary(b []byte) error {
	if len(b) < IPv6HeaderLen {
		return errors.New("short ipv6 header")
	}
	var (
		u8   byte
		u16  [2]byte
		u32  [4]byte
		u128 [16]byte
	)
	buf := bytes.NewBuffer(b)
	buf.Read(u32[:])
	h.Version = int(u32[0] >> 4)
	h.TrafficClass = int(u32[0]&0xf)<<4 | int(u32[1]>>4)
	h.FlowLabel = int(u32[1]&0xf)<<16 | int(u32[2])<<8 | int(u32[3])
	buf.Read(u16[:])
	h.PayloadLen = int(binary.BigEndian.Uint16(u16[:]))
	u8, _ = buf.ReadByte()
	h.NextHeader = IPProto(u8)
	u8, _ = buf.ReadByte()
	h.HopLimit = int(u8)
	buf.Read(u128[:])
	h.Src = append([]byte{}, u128[:]...)
	buf.Read(u128[:])
	h.Dst = append([]byte{}, u128[:]...)
	// payload
	if len(b) < h.PayloadLen && !h.IPinICMP {
		return errors.New("invalid IPv6 packet: payload too short")
	}
	payload := b[IPv6HeaderLen : IPv6HeaderLen+h.PayloadLen]
	if h.NextHeader == ProtoUDP {
		/*
			u, err := NewUDP(payload)
			if err != nil {
				return err
			}
			h.next = u
		*/
	} else {
		h.next = &Raw{Data: payload}
	}
	return nil
}
