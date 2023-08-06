/* SPDX-License-Identifier: BSD-2-Clause */

package net

/*
import (
	"bytes"
	"encoding/binary"
	"errors"
	"net"
)

// Version4 is IP version 4
var Version4 = 4

// MinIPv4HeaderLen is the minimum IPv4 header length
var MinIPv4HeaderLen = 20

// Flag is an IPv4 flag type
type Flag byte

// IPv4 flags
var (
	DontFragment  = 2
	MoreFragments = 4
)

// Option is an IP option
type Option [4]byte

// NewIPv4 constructs a new IPv4 header from a sequence of bytes
func NewIPv4(b []byte) (*IPv4, error) {
	var h IPv4
	if err := h.UnmarshalBinary(b); err != nil {
		return nil, err
	}
	return &h, nil
}

// IPv4 is the IPv4 header
type IPv4 struct {
	Version   int
	HeaderLen int
	DiffServ  int
	TotalLen  int
	ID        int
	Flags     int
	FragOff   int
	TTL       int
	Proto     IPProto
	Checksum  int
	Src       net.IP
	Dst       net.IP
	Options   []Option
	next      Layer
	// IP in ICMP, if set, won't make the parser fail on short packets
	IPinICMP bool
}

// Next returns the next layer
func (h IPv4) Next() Layer {
	return h.next
}

// SetNext sets the next layer
func (h *IPv4) SetNext(l Layer) {
	h.next = l
}

// MarshalBinary serializes the layer
func (h IPv4) MarshalBinary() ([]byte, error) {
	var b bytes.Buffer
	// Version check
	if h.Version == 0 {
		h.Version = Version4
	}
	if h.Version != Version4 {
		return nil, errors.New("invalid version")
	}
	// IHL checks
	h.HeaderLen = 5 + len(h.Options)
	if h.HeaderLen > 0xff || h.HeaderLen < 5 {
		return nil, errors.New("invalid ip header length")
	}
	binary.Write(&b, binary.BigEndian, byte(h.Version<<4)|byte(h.HeaderLen))

	// Differentiated Services (DSCP, ECN)
	if h.DiffServ < 0 || h.DiffServ > 0x3f {
		return nil, errors.New("invalid differentiated services")
	}
	binary.Write(&b, binary.BigEndian, byte(h.DiffServ))

	// Total length (header + data)
	// marshal the payload to know the length
	next := h.Next()
	var (
		payload []byte
		err     error
	)
	if next != nil {
		payload, err = next.MarshalBinary()
		if err != nil {
			return nil, err
		}
	}
	h.TotalLen = h.HeaderLen*4 + len(payload)
	if h.TotalLen < 0 || h.TotalLen > 0xffff {
		return nil, errors.New("invalid total length")
	}
	binary.Write(&b, binary.BigEndian, uint16(h.TotalLen))

	// ID
	if h.ID < 0 || h.ID > 0xffff {
		return nil, errors.New("invalid ID")
	}
	binary.Write(&b, binary.BigEndian, uint16(h.ID))

	// Flags
	if h.Flags < 0 || h.Flags > 0x7 || h.Flags&0x1 != 0 {
		return nil, errors.New("invalid flags")
	}
	var u16 = uint16(h.Flags << 13)

	// Fragment offset
	if h.FragOff < 0 || h.FragOff > 0x1fff {
		return nil, errors.New("invalid fragment offset")
	}
	u16 |= uint16(h.FragOff & 0x1fff)
	binary.Write(&b, binary.BigEndian, u16)

	// TTL
	if h.TTL < 0 || h.TTL > 0xff {
		return nil, errors.New("invalid TTL")
	}
	binary.Write(&b, binary.BigEndian, uint8(h.TTL))

	// Protocol
	if h.Proto < 0 || h.Proto > 0xff {
		return nil, errors.New("invalid protocol")
	}
		if h.Proto == 0 {
			switch next.(type) {
			case *UDP:
				h.Proto = ProtoUDP
			case *ICMP:
				h.Proto = ProtoICMP
			}
		}
	binary.Write(&b, binary.BigEndian, uint8(h.Proto))

	// Checksum - left to 0, filled in by the platform
	if h.Checksum < 0 || h.Checksum > 0xffff {
		return nil, errors.New("invalid checksum")
	}
	binary.Write(&b, binary.BigEndian, uint16(h.Checksum))

	// src and dst addresses
	if h.Src == nil {
		h.Src = net.IPv4zero
	}
	if h.Dst == nil {
		h.Dst = net.IPv4zero
	}
	binary.Write(&b, binary.BigEndian, h.Src.To4())
	binary.Write(&b, binary.BigEndian, h.Dst.To4())

	// Options
	for _, opt := range h.Options {
		binary.Write(&b, binary.BigEndian, opt)
	}

	ret := append(b.Bytes(), payload...)
	return ret, nil
}

// IsFragment returns whether this packet is a fragment of a larger packet.
func (h IPv4) IsFragment() bool {
	return h.Flags&MoreFragments != 0 || h.FragOff != 0
}

// UnmarshalBinary deserializes the raw bytes to an IPv4 header
func (h *IPv4) UnmarshalBinary(b []byte) error {
	if len(b) < MinIPv4HeaderLen {
		return errors.New("short ipv4 header")
	}
	var (
		u8  byte
		u16 [2]byte
		u32 [4]byte
	)
	buf := bytes.NewBuffer(b)
	u8, _ = buf.ReadByte()
	h.Version = int(u8 >> 4)
	if h.Version != Version4 {
		return errors.New("invalid version")
	}
	h.HeaderLen = int(u8 & 0xf)
	if len(b) < h.HeaderLen*4 {
		return errors.New("short ipv4 header")
	}
	u8, _ = buf.ReadByte()
	h.DiffServ = int(u8)
	buf.Read(u16[:])
	h.TotalLen = int(binary.BigEndian.Uint16(u16[:]))
	buf.Read(u16[:])
	h.ID = int(binary.BigEndian.Uint16(u16[:]))
	buf.Read(u16[:])
	tmp := binary.BigEndian.Uint16(u16[:])
	h.Flags = int((tmp >> 13) & 0x1f)
	h.FragOff = int(tmp & 0x1fff)
	u8, _ = buf.ReadByte()
	h.TTL = int(u8)
	u8, _ = buf.ReadByte()
	h.Proto = IPProto(u8)
	buf.Read(u16[:])
	h.Checksum = int(binary.BigEndian.Uint16(u16[:]))
	buf.Read(u32[:])
	h.Src = append([]byte{}, u32[:]...)
	buf.Read(u32[:])
	h.Dst = append([]byte{}, u32[:]...)
	h.Options = make([]Option, 0, h.HeaderLen-5)
	for i := 0; i < cap(h.Options); i++ {
		buf.Read(u32[:])
		h.Options = append(h.Options, Option(u32))
	}
	// payload
	if len(b) < h.TotalLen && !h.IPinICMP {
		return errors.New("invalid IPv4 packet: payload too short")
	}
	payload := b[h.HeaderLen*4 : h.TotalLen]
	if h.Proto == ProtoUDP && !h.IsFragment() {
			u, err := NewUDP(payload)
			if err != nil {
				return err
			}
				h.next = u
	} else if h.Proto == ProtoICMP {
		i, err := NewICMP(payload)
		if err != nil {
			return err
		}
		h.next = i
	} else {
		h.next = &Raw{Data: payload}
	}

	return nil
}
*/
