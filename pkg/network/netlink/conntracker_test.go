// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package netlink

import (
	"crypto/rand"
	"net/netip"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sys/unix"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func TestIsNat(t *testing.T) {
	src := "1.1.1.1"
	dst := "2.2.2.2"
	tdst := "3.3.3.3"
	var srcPort uint16 = 42
	var dstPort uint16 = 8080

	t.Run("not nat", func(t *testing.T) {
		c := Con{
			Origin: newIPTuple(src, dst, srcPort, dstPort, uint8(unix.IPPROTO_TCP)),
			Reply:  newIPTuple(dst, src, dstPort, srcPort, uint8(unix.IPPROTO_TCP)),
		}
		assert.False(t, IsNAT(c))
	})

	t.Run("zero proto field", func(t *testing.T) {
		c := Con{
			Origin: newIPTuple(src, dst, 0, 0, 0),
			Reply:  newIPTuple(dst, src, 0, 0, 0),
		}
		assert.False(t, IsNAT(c))
	})

	t.Run("zero src port field", func(t *testing.T) {
		c := Con{
			Origin: newIPTuple(src, dst, 0, dstPort, uint8(unix.IPPROTO_TCP)),
			Reply:  newIPTuple(dst, src, 0, srcPort, uint8(unix.IPPROTO_TCP)),
		}
		assert.False(t, IsNAT(c))
	})
	t.Run("zero dst port field", func(t *testing.T) {
		c := Con{
			Origin: newIPTuple(src, dst, srcPort, 0, uint8(unix.IPPROTO_TCP)),
			Reply:  newIPTuple(dst, src, dstPort, 0, uint8(unix.IPPROTO_TCP)),
		}
		assert.False(t, IsNAT(c))
	})

	t.Run("nat", func(t *testing.T) {
		c := Con{
			Origin: newIPTuple(src, dst, srcPort, dstPort, uint8(unix.IPPROTO_TCP)),
			Reply:  newIPTuple(tdst, src, dstPort, srcPort, uint8(unix.IPPROTO_TCP)),
		}
		assert.True(t, IsNAT(c))
	})
}

func TestRegisterNonNat(t *testing.T) {
	rt := newConntracker(10000)
	c := makeUntranslatedConn(netip.MustParseAddr("10.0.0.0"), netip.MustParseAddr("50.30.40.10"), 6, 8080, 12345)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		&network.ConnectionTuple{
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
	c := makeTranslatedConn(netip.MustParseAddr("10.0.0.0"), netip.MustParseAddr("20.0.0.0"), netip.MustParseAddr("50.30.40.10"), 6, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		&network.ConnectionTuple{
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
		&network.ConnectionTuple{
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
	c := makeTranslatedConn(netip.MustParseAddr("10.0.0.0"), netip.MustParseAddr("20.0.0.0"), netip.MustParseAddr("50.30.40.10"), 17, 12345, 80, 80)

	rt.register(c)
	translation := rt.GetTranslationForConn(
		&network.ConnectionTuple{
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
		&network.ConnectionTuple{
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

	rt.register(makeTranslatedConn(netip.MustParseAddr("10.0.0.0"), netip.MustParseAddr("20.0.0.0"), netip.MustParseAddr("50.30.40.10"), 6, 12345, 80, 80))
	tr := rt.GetTranslationForConn(&network.ConnectionTuple{
		Source: util.AddressFromString("10.0.0.0"),
		SPort:  12345,
		Dest:   util.AddressFromString("50.30.40.10"),
		DPort:  80,
		Type:   network.TCP,
	})
	require.NotNil(t, tr)
	require.Equal(t, "20.0.0.0", tr.ReplSrcIP.String())
	require.Equal(t, uint16(80), tr.ReplSrcPort)

	rt.register(makeTranslatedConn(netip.MustParseAddr("10.0.0.1"), netip.MustParseAddr("20.0.0.1"), netip.MustParseAddr("50.30.40.20"), 6, 12345, 80, 80))
	// old entry should be gone
	tr = rt.GetTranslationForConn(&network.ConnectionTuple{
		Source: util.AddressFromString("10.0.0.0"),
		SPort:  12345,
		Dest:   util.AddressFromString("50.30.40.10"),
		DPort:  80,
		Type:   network.TCP,
	})
	require.Nil(t, tr)

	// check new entry
	tr = rt.GetTranslationForConn(&network.ConnectionTuple{
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
func TestConntrackerMemoryAllocation(_t *testing.T) {
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
				netip.MustParseAddr("1.1.1.1"),
				netip.MustParseAddr("2.2.2.2"),
				netip.MustParseAddr("3.3.3.3"),
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
				netip.MustParseAddr("1.1.1.1"),
				netip.MustParseAddr("2.2.2.2"),
				netip.MustParseAddr("3.3.3.3"),
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
					src: netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 12345),
					dst: netip.AddrPortFrom(netip.MustParseAddr("3.3.3.3"), 80),
				},
				expectedReplSrcIP:   "2.2.2.2",
				expectedReplSrcPort: 80,
			},
			{
				k: connKey{
					src: netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 80),
					dst: netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 12345),
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
		}

		for _, te := range tests {
			tr, ok := cache.cache.Get(te.k)
			require.True(t, ok, "translation entry not found for key %+v", te.k)
			require.NotNil(t, tr)
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
				netip.MustParseAddr("1.1.1.1"),
				netip.MustParseAddr("2.2.2.2"),
				netip.MustParseAddr("3.3.3.3"),
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
				netip.MustParseAddr("1.1.1.1"),
				netip.MustParseAddr("4.4.4.4"),
				netip.MustParseAddr("3.3.3.3"),
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
					src: netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 12345),
					dst: netip.AddrPortFrom(netip.MustParseAddr("3.3.3.3"), 80),
				},
				expectedReplSrcIP:   "4.4.4.4",
				expectedReplSrcPort: 80,
			},
			{
				k: connKey{
					src: netip.AddrPortFrom(netip.MustParseAddr("4.4.4.4"), 80),
					dst: netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 12345),
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
			{
				k: connKey{
					src: netip.AddrPortFrom(netip.MustParseAddr("2.2.2.2"), 80),
					dst: netip.AddrPortFrom(netip.MustParseAddr("1.1.1.1"), 12345),
				},
				expectedReplSrcIP:   "1.1.1.1",
				expectedReplSrcPort: 12345,
			},
		}

		for _, te := range tests {
			tr, ok := cache.cache.Get(te.k)
			require.True(t, ok, "translation entry not found for key %+v", te.k)
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
		require.Equal(t, l, v.orphan)
	}
}

func newConntracker(maxSize int) *realConntracker {
	rt := &realConntracker{
		maxStateSize: maxSize,
		cache:        newConntrackCache(maxSize, defaultOrphanTimeout),
	}

	return rt
}

func makeUntranslatedConn(src, dst netip.Addr, proto uint8, srcPort, dstPort uint16) Con {
	return makeTranslatedConn(src, dst, dst, proto, srcPort, dstPort, dstPort)
}

// makes a translation where from -> to is shows as transFrom -> from
func makeTranslatedConn(from, transFrom, to netip.Addr, proto uint8, fromPort, transFromPort, toPort uint16) Con {
	return Con{
		Origin: ConTuple{
			Src:   netip.AddrPortFrom(from, fromPort),
			Dst:   netip.AddrPortFrom(to, toPort),
			Proto: proto,
		},
		Reply: ConTuple{
			Src:   netip.AddrPortFrom(transFrom, transFromPort),
			Dst:   netip.AddrPortFrom(from, fromPort),
			Proto: proto,
		},
	}
}

func randomIPGen() func() netip.Addr {
	var b [4]byte
	return func() netip.Addr {
		for {
			if _, err := rand.Read(b[:]); err != nil {
				continue
			}

			return netip.AddrFrom4(b)
		}
	}
}
