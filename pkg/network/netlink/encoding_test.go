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

	decoder := NewDecoder()
	connections := decoder.DecodeAndReleaseEvent(Event{
		msgs: []netlink.Message{
			{
				Data: data,
			},
		},
	})
	require.Len(t, connections, 1)
	c := connections[0]

	assert.True(t, conn.Con.Origin.Src.Equal(*c.Origin.Src))
	assert.True(t, conn.Con.Origin.Dst.Equal(*c.Origin.Dst))

	assert.Equal(t, *conn.Con.Origin.Proto.SrcPort, *c.Origin.Proto.SrcPort)
	assert.Equal(t, *conn.Con.Origin.Proto.DstPort, *c.Origin.Proto.DstPort)
	assert.Equal(t, *conn.Con.Origin.Proto.Number, *c.Origin.Proto.Number)

	assert.True(t, conn.Con.Reply.Src.Equal(*c.Reply.Src))
	assert.True(t, conn.Con.Reply.Dst.Equal(*c.Reply.Dst))

	assert.Equal(t, *conn.Con.Reply.Proto.SrcPort, *c.Reply.Proto.SrcPort)
	assert.Equal(t, *conn.Con.Reply.Proto.DstPort, *c.Reply.Proto.DstPort)
	assert.Equal(t, *conn.Con.Reply.Proto.Number, *c.Reply.Proto.Number)

}
