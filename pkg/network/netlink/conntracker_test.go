// +build linux
// +build !android

package netlink

import (
	"crypto/rand"
	"net"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network"
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

		c := Con{
			ct.Con{
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
			},
			0,
		}
		assert.False(t, IsNAT(c))
	})

	t.Run("nil proto field", func(t *testing.T) {
		c := Con{
			ct.Con{
				Origin: &ct.IPTuple{
					Src: &src,
					Dst: &dst,
				},
				Reply: &ct.IPTuple{
					Src: &dst,
					Dst: &src,
				},
			},
			0,
		}
		assert.False(t, IsNAT(c))
	})

	t.Run("nat", func(t *testing.T) {

		c := Con{
			ct.Con{
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
			},
			0,
		}
		assert.True(t, IsNAT(c))
	})
}

func TestRegisterNonNat(t *testing.T) {
	rt := newConntracker(10000)
	c := makeUntranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("50.30.40.10"), 6, 8080, 12345)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.0"),
			SPort:  8080,
			Dest:   util.AddressFromString("50.30.40.10"),
			DPort:  12345,
			Type:   network.TCP,
		},
	)
	assert.Nil(t, translation)
}

func TestRegisterNat(t *testing.T) {
	rt := newConntracker(10000)
	c := makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.0"),
			SPort:  12345,
			Dest:   util.AddressFromString("50.30.40.10"),
			DPort:  80,
			Type:   network.TCP,
		},
	)
	assert.NotNil(t, translation)
	assert.Equal(t, &network.IPTranslation{
		ReplSrcIP:   util.AddressFromString("20.0.0.0"),
		ReplDstIP:   util.AddressFromString("10.0.0.0"),
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	udpTranslation := rt.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.0"),
			SPort:  12345,
			Dest:   util.AddressFromString("50.30.40.10"),
			DPort:  80,
			Type:   network.UDP,
		},
	)
	assert.Nil(t, udpTranslation)

}

func TestRegisterNatUDP(t *testing.T) {
	rt := newConntracker(10000)
	c := makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 17, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.0"),
			SPort:  12345,
			Dest:   util.AddressFromString("50.30.40.10"),
			DPort:  80,
			Type:   network.UDP,
		},
	)
	assert.NotNil(t, translation)
	assert.Equal(t, &network.IPTranslation{
		ReplSrcIP:   util.AddressFromString("20.0.0.0"),
		ReplDstIP:   util.AddressFromString("10.0.0.0"),
		ReplSrcPort: 80,
		ReplDstPort: 12345,
	}, translation)

	translation = rt.GetTranslationForConn(
		network.ConnectionStats{
			Source: util.AddressFromString("10.0.0.0"),
			SPort:  12345,
			Dest:   util.AddressFromString("50.30.40.10"),
			DPort:  80,
			Type:   network.TCP,
		},
	)
	assert.Nil(t, translation)
}

func TestTooManyEntries(t *testing.T) {
	rt := newConntracker(2)

	rt.register(makeTranslatedConn(net.ParseIP("10.0.0.0"), net.ParseIP("20.0.0.0"), net.ParseIP("50.30.40.10"), 6, 12345, 80, 80))
	tr := rt.GetTranslationForConn(network.ConnectionStats{
		Source: util.AddressFromString("10.0.0.0"),
		SPort:  12345,
		Dest:   util.AddressFromString("50.30.40.10"),
		DPort:  80,
		Type:   network.TCP,
	})
	require.NotNil(t, tr)
	require.Equal(t, "20.0.0.0", tr.ReplSrcIP.String())
	require.Equal(t, uint16(80), tr.ReplSrcPort)

	rt.register(makeTranslatedConn(net.ParseIP("10.0.0.1"), net.ParseIP("20.0.0.1"), net.ParseIP("50.30.40.20"), 6, 12345, 80, 80))
	// old entry should be gone
	tr = rt.GetTranslationForConn(network.ConnectionStats{
		Source: util.AddressFromString("10.0.0.0"),
		SPort:  12345,
		Dest:   util.AddressFromString("50.30.40.10"),
		DPort:  80,
		Type:   network.TCP,
	})
	require.Nil(t, tr)

	// check new entry
	tr = rt.GetTranslationForConn(network.ConnectionStats{
		Source: util.AddressFromString("10.0.0.1"),
		SPort:  12345,
		Dest:   util.AddressFromString("50.30.40.20"),
		DPort:  80,
		Type:   network.TCP,
	})
	require.NotNil(t, tr)
	require.Equal(t, "20.0.0.1", tr.ReplSrcIP.String())
	require.Equal(t, uint16(80), tr.ReplSrcPort)
}

// Run this test with -memprofile to get an insight of how much memory is
// allocated/used by Conntracker to store maxStateSize entries.
// Example: go test -run TestConntrackerMemoryAllocation -memprofile mem.prof .
func TestConntrackerMemoryAllocation(t *testing.T) {
	rt := newConntracker(10000)
	ipGen := randomIPGen()

	for i := 0; i < rt.maxStateSize; i++ {
		c := makeTranslatedConn(ipGen(), ipGen(), ipGen(), 6, 12345, 80, 80)
		rt.register(c)
	}
}

func TestConntrackCacheAdd(t *testing.T) {
	t.Run("orphan false", func(t *testing.T) {
		cache := newConntrackCache(10, defaultOrphanTimeout)
		cache.Add(
			makeTranslatedConn(
				net.ParseIP("1.1.1.1"),
				net.ParseIP("2.2.2.2"),
				net.ParseIP("3.3.3.3"),
				6,
				12345,
				80,
				80),
			false)
		require.Equal(t, 2, cache.cache.Len())
		require.Equal(t, 0, cache.orphans.Len())
		crossCheckCacheOrphans(t, cache)
	})

	t.Run("orphan true", func(t *testing.T) {
		cache := newConntrackCache(10, defaultOrphanTimeout)
		cache.Add(
			makeTranslatedConn(
				net.ParseIP("1.1.1.1"),
				net.ParseIP("2.2.2.2"),
				net.ParseIP("3.3.3.3"),
				6,
				12345,
				80,
				80),
			true)
		require.Equal(t, 2, cache.cache.Len())
		require.Equal(t, 2, cache.orphans.Len())
		crossCheckCacheOrphans(t, cache)

		tests := []struct {
			k                   connKey
			expectedReplSrcIP   string
			expectedReplSrcPort uint16
		}{
			{
				k: connKey{
					srcIP:   util.AddressFromString("1.1.1.1"),
					srcPort: 12345,
					dstIP:   util.AddressFromString("3.3.3.3"),
					dstPort: 80,
				},
				expectedReplSrcIP:   "2.2.2.2",
				expectedReplSrcPort: 80,
			},
			{
				k: connKey{
					srcIP:   util.AddressFromString("2.2.2.2"),
					srcPort: 80,
					dstIP:   util.AddressFromString("1.1.1.1"),
					dstPort: 12345,
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
		}

		for _, te := range tests {
			v, ok := cache.cache.Get(te.k)
			require.True(t, ok, "translation entry not found for key %+v", te.k)
			require.NotNil(t, v)
			tr := v.(*translationEntry)
			require.Equal(t, te.expectedReplSrcIP, tr.IPTranslation.ReplSrcIP.String())
			require.Equal(t, te.expectedReplSrcPort, tr.IPTranslation.ReplSrcPort)
			require.NotNil(t, tr.orphan)
			o := tr.orphan.Value.(*orphanEntry)
			require.Equal(t, te.k, o.key)
			// only way to check if tr.orphan is in
			// rt.orphans is to remove it from
			// rt.orphans
			ol := cache.orphans.Len()
			cache.orphans.Remove(tr.orphan)
			require.Equal(t, ol-1, cache.orphans.Len())
		}
	})

	t.Run("orphan true, existing key", func(t *testing.T) {
		cache := newConntrackCache(10, defaultOrphanTimeout)
		cache.Add(
			makeTranslatedConn(
				net.ParseIP("1.1.1.1"),
				net.ParseIP("2.2.2.2"),
				net.ParseIP("3.3.3.3"),
				6,
				12345,
				80,
				80),
			true)
		require.Equal(t, 2, cache.cache.Len())
		require.Equal(t, 2, cache.orphans.Len())
		crossCheckCacheOrphans(t, cache)

		// add a connection with the same origin
		// values but different reply
		cache.Add(
			makeTranslatedConn(
				net.ParseIP("1.1.1.1"),
				net.ParseIP("4.4.4.4"),
				net.ParseIP("3.3.3.3"),
				6,
				12345,
				80,
				80),
			true)
		require.Equal(t, 3, cache.cache.Len())
		require.Equal(t, 3, cache.orphans.Len())
		crossCheckCacheOrphans(t, cache)

		tests := []struct {
			k                   connKey
			expectedReplSrcIP   string
			expectedReplSrcPort uint16
		}{
			{
				k: connKey{
					srcIP:   util.AddressFromString("1.1.1.1"),
					srcPort: 12345,
					dstIP:   util.AddressFromString("3.3.3.3"),
					dstPort: 80,
				},
				expectedReplSrcIP:   "4.4.4.4",
				expectedReplSrcPort: 80,
			},
			{
				k: connKey{
					srcIP:   util.AddressFromString("4.4.4.4"),
					srcPort: 80,
					dstIP:   util.AddressFromString("1.1.1.1"),
					dstPort: 12345,
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
			{
				k: connKey{
					srcIP:   util.AddressFromString("2.2.2.2"),
					srcPort: 80,
					dstIP:   util.AddressFromString("1.1.1.1"),
					dstPort: 12345,
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
		}

		for _, te := range tests {
			v, ok := cache.cache.Get(te.k)
			require.True(t, ok, "translation entry not found for key %+v", te.k)
			tr := v.(*translationEntry)
			require.Equal(t, te.expectedReplSrcIP, tr.IPTranslation.ReplSrcIP.String())
			require.Equal(t, te.expectedReplSrcPort, tr.IPTranslation.ReplSrcPort)
			require.NotNil(t, tr.orphan)
			o := tr.orphan.Value.(*orphanEntry)
			require.Equal(t, te.k, o.key)
			// only way to check if tr.orphan is in
			// rt.orphans is to remove it from
			// rt.orphans
			ol := cache.orphans.Len()
			cache.orphans.Remove(tr.orphan)
			require.Equal(t, ol-1, cache.orphans.Len())
		}
	})
}

func TestConntrackCacheRemoveOrphans(t *testing.T) {
	t.Run("empty orphans list", func(t *testing.T) {
		rt := newConntracker(10)
		rt.cache.orphanTimeout = defaultOrphanTimeout

		require.Equal(t, int64(0), rt.cache.removeOrphans(time.Now().Add(rt.cache.orphanTimeout).Add(time.Second)))
	})

	t.Run("all orphans expired", func(t *testing.T) {
		rt := newConntracker(20)
		rt.cache.orphanTimeout = defaultOrphanTimeout

		ipGen := randomIPGen()
		for i := 0; i < rt.maxStateSize/2; i++ {
			c := makeTranslatedConn(ipGen(), ipGen(), ipGen(), 6, 12345, 80, 80)
			rt.register(c)
		}

		require.Equal(t, int64(rt.maxStateSize), rt.cache.removeOrphans(time.Now().Add(rt.cache.orphanTimeout).Add(time.Minute)))
		require.Equal(t, 0, rt.cache.orphans.Len())
		require.Equal(t, 0, rt.cache.cache.Len())
		crossCheckCacheOrphans(t, rt.cache)
	})

	t.Run("partial orphans expired", func(t *testing.T) {
		rt := newConntracker(20)
		ipGen := randomIPGen()

		rt.cache.orphanTimeout = time.Second
		for i := 0; i < rt.maxStateSize/4; i++ {
			c := makeTranslatedConn(ipGen(), ipGen(), ipGen(), 6, 12345, 80, 80)
			rt.register(c)
		}

		rt.cache.orphanTimeout = time.Minute
		for i := 0; i < rt.maxStateSize/4; i++ {
			c := makeTranslatedConn(ipGen(), ipGen(), ipGen(), 6, 12345, 80, 80)
			rt.register(c)
		}

		require.Equal(t, int64(rt.maxStateSize/2), rt.cache.removeOrphans(time.Now().Add(5*time.Second)))
		require.Equal(t, rt.maxStateSize/2, rt.cache.orphans.Len())
		require.Equal(t, rt.maxStateSize/2, rt.cache.cache.Len())
		crossCheckCacheOrphans(t, rt.cache)

		require.Equal(t, int64(rt.maxStateSize/2), rt.cache.removeOrphans(time.Now().Add(2*time.Minute)))
		require.Equal(t, 0, rt.cache.orphans.Len())
		require.Equal(t, 0, rt.cache.cache.Len())
		crossCheckCacheOrphans(t, rt.cache)
	})

}

func crossCheckCacheOrphans(t *testing.T, cc *conntrackCache) {
	for l := cc.orphans.Front(); l != nil; l = l.Next() {
		o := l.Value.(*orphanEntry)
		v, ok := cc.cache.Get(o.key)
		require.True(t, ok)
		require.Equal(t, l, v.(*translationEntry).orphan)
	}
}

func newConntracker(maxSize int) *realConntracker {
	rt := &realConntracker{
		maxStateSize: maxSize,
		cache:        newConntrackCache(maxSize, defaultOrphanTimeout),
	}

	return rt
}

func makeUntranslatedConn(src, dst net.IP, proto uint8, srcPort, dstPort uint16) Con {
	return makeTranslatedConn(src, dst, dst, proto, srcPort, dstPort, dstPort)
}

// makes a translation where from -> to is shows as transFrom -> from
func makeTranslatedConn(from, transFrom, to net.IP, proto uint8, fromPort, transFromPort, toPort uint16) Con {

	return Con{
		ct.Con{
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
		},
		0,
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
