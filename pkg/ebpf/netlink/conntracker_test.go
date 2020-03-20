// +build linux

package netlink

import (
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/DataDog/agent-payload/process"
	"github.com/DataDog/datadog-agent/pkg/process/util"
	ct "github.com/florianl/go-conntrack"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNat(t *testing.T) {
	src := net.ParseIP("1.1.1.1")
	dst := net.ParseIP("2.2.2..2")
	tdst := net.ParseIP("3.3.3.3")
	var srcPort uint16 = 42
	var dstPort uint16 = 8080

	t.Run("not nat", func(t *testing.T) {

		c := ct.Con{
			Origin: &ct.IPTuple{
				Src: &src,
				Dst: &dst,
				Proto: &ct.ProtoTuple{
					SrcPort: &srcPort,
					DstPort: &dstPort,
				},
			},
			Reply: &ct.IPTuple{
				Src: &dst,
				Dst: &src,
				Proto: &ct.ProtoTuple{
					SrcPort: &dstPort,
					DstPort: &srcPort,
				},
			},
		}
		assert.False(t, isNAT(c))
	})

	t.Run("nil proto field", func(t *testing.T) {
		c := ct.Con{
			Origin: &ct.IPTuple{
				Src: &src,
				Dst: &dst,
			},
			Reply: &ct.IPTuple{
				Src: &dst,
				Dst: &src,
			},
		}
		assert.False(t, isNAT(c))
	})

	t.Run("nat", func(t *testing.T) {

		c := ct.Con{
			Origin: &ct.IPTuple{
				Src: &src,
				Dst: &dst,
				Proto: &ct.ProtoTuple{
					SrcPort: &srcPort,
					DstPort: &dstPort,
				},
			},
			Reply: &ct.IPTuple{
				Src: &tdst,
				Dst: &src,
				Proto: &ct.ProtoTuple{
					SrcPort: &dstPort,
					DstPort: &srcPort,
				},
			},
		}
		assert.True(t, isNAT(c))
	})
}

func TestRegisterNonNat(t *testing.T) {
	rt := newConntracker()
	c := makeUntranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("50.30.40.10"), 6, 8080, 12345)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 8080,
		util.AddressFromString("50.30.40.10"), 12345,
		process.ConnectionType_tcp,
	)
	assert.Nil(t, translation)
}

func TestRegisterNat(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_tcp,
	)
	assert.NotNil(t, translation)
	assert.Equal(t, &IPTranslation{
		ReplSrcIP:   util.AddressFromString("20.0.0.0"),
		ReplDstIP:   util.AddressFromString("10.0.0.0"),
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	udpTranslation := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_udp,
	)
	assert.Nil(t, udpTranslation)

	// even after unregistering, we should be able to access the conn
	rt.unregister(c)
	translation2 := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_tcp,
	)
	assert.NotNil(t, translation2)

	// double unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	rt.ClearShortLived()
	translation3 := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_tcp,
	)
	assert.Nil(t, translation3)

	// triple unregisters should never happen, though it will be treated as a no-op
	rt.unregister(c)

	assert.Equal(t, translation, translation2)

}

func TestRegisterNatUDP(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 17, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_udp,
	)
	assert.NotNil(t, translation)
	assert.Equal(t, &IPTranslation{
		ReplSrcIP:   util.AddressFromString("20.0.0.0"),
		ReplDstIP:   util.AddressFromString("10.0.0.0"),
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	translation = rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_tcp,
	)
	assert.Nil(t, translation)
}

func TestGetUpdatesGen(t *testing.T) {
	rt := newConntracker()
	c := makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80)

	rt.register(c)
	var last uint8

	// there will be two entries in rt.state for the entry we just registered.
	// set them both to be 5 generations older than they are
	require.Len(t, rt.state, 2)
	for _, v := range rt.state {
		v.expGeneration -= 5
		last = v.expGeneration
	}

	iptr := rt.GetTranslationForConn(
		util.AddressFromString("10.0.0.0"), 12345,
		util.AddressFromString("50.30.40.10"), 80,
		process.ConnectionType_tcp,
	)
	require.NotNil(t, iptr)

	require.Len(t, rt.state, 2)
	require.Len(t, rt.shortLivedBuffer, 0)
	entry := rt.state[connKey{
		srcIP: util.AddressFromString("10.0.0.0"), srcPort: 12345,
		dstIP: util.AddressFromString("50.30.40.10"), dstPort: 80,
		transport: process.ConnectionType_tcp,
	}]
	assert.NotEqual(t, entry.expGeneration, last, "expected %v to equal %v", entry.expGeneration, last)
}

func TestTooManyEntries(t *testing.T) {
	rt := newConntracker()
	rt.maxStateSize = 1

	rt.register(makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80))
	rt.register(makeTranslatedConn(net.ParseIP("10.0.0.1"), net.ParseIP("20.0.0.1"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80))
	rt.register(makeTranslatedConn(net.ParseIP("10.0.0.2"), net.ParseIP("20.0.0.2"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80))
}

// Run this test with -memprofile to get an insight of how much memory is
// allocated/used by Conntracker to store maxStateSize entries.
// Example: go test -run TestConntrackerMemoryAllocation -memprofile mem.prof .
func TestConntrackerMemoryAllocation(t *testing.T) {
	rt := newConntracker()
	ipGen := randomIPGen()

	for i := 0; i < rt.maxStateSize; i++ {
		c := makeTranslatedConn(ipGen(), ipGen(), ipGen(), 6, 12345, 80, 80)
		rt.register(c)
	}
}

func newConntracker() *realConntracker {
	return &realConntracker{
		state:                make(map[connKey]*connValue),
		shortLivedBuffer:     make(map[connKey]*IPTranslation),
		maxShortLivedBuffer:  10000,
		compactTicker:        time.NewTicker(time.Hour),
		maxStateSize:         10000,
		exceededSizeLogLimit: util.NewLogLimit(1, time.Minute),
	}
}

func makeUntranslatedConn(src, dst net.IP, proto uint8, srcPort, dstPort uint16) ct.Con {
	return makeTranslatedConn(src, dst, dst, proto, srcPort, dstPort, dstPort)
}

// makes a translation where from -> to is shows as transFrom -> from
func makeTranslatedConn(from, transFrom, to net.IP, proto uint8, fromPort, transFromPort, toPort uint16) ct.Con {

	return ct.Con{
		Origin: &ct.IPTuple{
			Src: &from,
			Dst: &to,
			Proto: &ct.ProtoTuple{
				Number:  &proto,
				SrcPort: &fromPort,
				DstPort: &toPort,
			},
		},
		Reply: &ct.IPTuple{
			Src: &transFrom,
			Dst: &from,
			Proto: &ct.ProtoTuple{
				Number:  &proto,
				SrcPort: &transFromPort,
				DstPort: &fromPort,
			},
		},
	}
}

func randomIPGen() func() net.IP {
	b := make([]byte, 4)
	return func() net.IP {
		for {
			if _, err := rand.Read(b); err != nil {
				continue
			}

			return net.IPv4(b[0], b[1], b[2], b[3])
		}
	}
}
