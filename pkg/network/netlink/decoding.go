// +build linux
// +build !android

package netlink

import (
	"encoding/binary"
	"fmt"
	"net"

	"github.com/DataDog/datadog-agent/pkg/util/log"
	ct "github.com/florianl/go-conntrack"
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

// Con represents a conntrack entry, along with any network namespace info (nsid)
type Con struct {
	ct.Con
	NetNS int32
}

func (c Con) String() string {
	return fmt.Sprintf("netns=%d src=%s dst=%s sport=%d dport=%d src=%s dst=%s sport=%d dport=%d proto=%d", c.NetNS, c.Origin.Src, c.Origin.Dst, *c.Origin.Proto.SrcPort, *c.Origin.Proto.DstPort, c.Reply.Src, c.Reply.Dst, *c.Reply.Proto.SrcPort, *c.Reply.Proto.DstPort, *c.Con.Origin.Proto.Number)
}

var scanner = NewAttributeScanner()

// DecodeAndReleaseEvent decodes a single Event into a slice of []ct.Con objects and
// releases the underlying buffer.
// TODO: Replace the intermediate ct.Con object by the same format we use in the cache
func DecodeAndReleaseEvent(e Event) []Con {
	msgs := e.Messages()
	conns := make([]Con, 0, len(msgs))

	for _, msg := range msgs {
		c := &Con{NetNS: e.netns}
		if err := scanner.ResetTo(msg.Data); err != nil {
			log.Debugf("error decoding netlink message: %s", err)
			continue
		}
		err := unmarshalCon(scanner, c)
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

func unmarshalCon(s *AttributeScanner, c *Con) error {
	c.Origin = &ct.IPTuple{}
	c.Reply = &ct.IPTuple{}

	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaTupleOrig:
			toDecode--
			s.Nested(func() error {
				return unmarshalTuple(s, c.Origin)
			})
		case ctaTupleReply:
			toDecode--
			s.Nested(func() error {
				return unmarshalTuple(s, c.Reply)
			})
		}
	}

	return s.Err()
}

func unmarshalTuple(s *AttributeScanner, t *ct.IPTuple) error {
	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaTupleIP:
			toDecode--
			s.Nested(func() error {
				return unmarshalTupleIP(s, t)
			})
		case ctaTupleProto:
			toDecode--
			s.Nested(func() error {
				return unmarshalProto(s, t)
			})
		}
	}
	return s.Err()
}

// We might also want to consider deferring the allocation of the IP byte slice
func unmarshalTupleIP(s *AttributeScanner, t *ct.IPTuple) error {
	for toDecode := 2; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaIPv4Src, ctaIPv6Src:
			toDecode--
			data := copySlice(s.Bytes())
			ip := net.IP(data)
			t.Src = &ip
		case ctaIPv4Dst, ctaIPv6Dst:
			toDecode--
			data := copySlice(s.Bytes())
			ip := net.IP(data)
			t.Dst = &ip
		}
	}

	return s.Err()
}

func unmarshalProto(s *AttributeScanner, t *ct.IPTuple) error {
	t.Proto = &ct.ProtoTuple{}

	for toDecode := 3; toDecode > 0 && s.Next(); {
		switch s.Type() {
		case ctaProtoNum:
			toDecode--
			protoNum := s.Bytes()[0]
			t.Proto.Number = &protoNum
		case ctaProtoSrcPort:
			toDecode--
			port := binary.BigEndian.Uint16(s.Bytes())
			t.Proto.SrcPort = &port
		case ctaProtoDstPort:
			toDecode--
			port := binary.BigEndian.Uint16(s.Bytes())
			t.Proto.DstPort = &port
		}
	}

	return s.Err()
}

func copySlice(src []byte) []byte {
	dst := make([]byte, len(src))
	copy(dst, src)
	return dst
}
