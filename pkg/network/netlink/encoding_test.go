// +build linux
// +build !android

package netlink

import (
	"net"
	"testing"

	ct "github.com/florianl/go-conntrack"
	"github.com/mdlayher/netlink"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"
)

func newIPTuple(srcIP, dstIP string, srcPort, dstPort uint16, proto uint8) *ct.IPTuple {
	_s := net.ParseIP(srcIP)
	_d := net.ParseIP(dstIP)
	return &ct.IPTuple{
		Src: &_s,
		Dst: &_d,
		Proto: &ct.ProtoTuple{
			Number:  &proto,
			SrcPort: &srcPort,
			DstPort: &dstPort,
		},
	}
}

func TestEncodeConn(t *testing.T) {
	// orig_src=10.0.2.15:58472 orig_dst=2.2.2.2:5432 reply_src=1.1.1.1:5432 reply_dst=10.0.2.15:58472 proto=tcp(6)
	origin := newIPTuple("10.0.2.15", "2.2.2.2", 58472, 5432, uint8(unix.IPPROTO_TCP))
	reply := newIPTuple("1.1.1.1", "10.0.2.15", 5432, 58472, uint8(unix.IPPROTO_TCP))
	conn := Con{
		Con: ct.Con{
			Origin: origin,
			Reply:  reply,
		},
	}

	data, err := EncodeConn(&conn)
	require.NoError(t, err)
	require.NotEmpty(t, data)

	connections := DecodeAndReleaseEvent(Event{
		msgs: []netlink.Message{
			{
				Data: data,
			},
		},
	})
	require.Len(t, connections, 1)
	c := connections[0]

	assert.True(t, net.ParseIP("10.0.2.15").Equal(*c.Origin.Src))
	assert.True(t, net.ParseIP("2.2.2.2").Equal(*c.Origin.Dst))

	assert.Equal(t, uint16(58472), *c.Origin.Proto.SrcPort)
	assert.Equal(t, uint16(5432), *c.Origin.Proto.DstPort)
	assert.Equal(t, uint8(unix.IPPROTO_TCP), *c.Origin.Proto.Number)

	assert.True(t, net.ParseIP("1.1.1.1").Equal(*c.Reply.Src))
	assert.True(t, net.ParseIP("10.0.2.15").Equal(*c.Reply.Dst))

	assert.Equal(t, uint16(5432), *c.Reply.Proto.SrcPort)
	assert.Equal(t, uint16(58472), *c.Reply.Proto.DstPort)
	assert.Equal(t, uint8(unix.IPPROTO_TCP), *c.Reply.Proto.Number)

}
