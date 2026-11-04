// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build (linux && bpf) || darwin

package connection

import (
	"encoding/binary"
	"math/rand"
	"net/netip"
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/twmb/murmur3"

	"github.com/DataDog/datadog-agent/pkg/network"
	"github.com/DataDog/datadog-agent/pkg/process/util"
)

func randomCookieConn(r *rand.Rand) network.ConnectionStats {
	c := network.ConnectionStats{Cookie: r.Uint64()}
	c.Pid = r.Uint32()
	c.SPort, c.DPort = uint16(r.Intn(65536)), uint16(r.Intn(65536))
	c.Type = network.ConnectionType(r.Intn(2))
	if r.Intn(2) == 0 {
		c.Family = network.AFINET
		c.Source = util.Address{Addr: netip.AddrFrom4([4]byte{byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256))})}
		c.Dest = util.Address{Addr: netip.AddrFrom4([4]byte{byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256)), byte(r.Intn(256))})}
	} else {
		var s, d [16]byte
		r.Read(s[:])
		r.Read(d[:])
		c.Family = network.AFINET6
		c.Source = util.Address{Addr: netip.AddrFrom16(s)}
		c.Dest = util.Address{Addr: netip.AddrFrom16(d)}
	}
	return c
}

// TestCookieHashMatchesStreamingHash checks the hash against the streaming murmur3 form it replaced.
func TestCookieHashMatchesStreamingHash(t *testing.T) {
	r := rand.New(rand.NewSource(1))
	h := newCookieHasher()
	keyBuf := make([]byte, network.ConnectionByteKeyMaxLen)
	for i := 0; i < 100000; i++ {
		c := randomCookieConn(r)

		ref := murmur3.New64()
		require.NoError(t, binary.Write(ref, binary.BigEndian, c.Cookie))
		_, err := ref.Write(c.ByteKey(keyBuf))
		require.NoError(t, err)

		h.Hash(&c)
		require.Equal(t, ref.Sum64(), c.Cookie)
	}
}

func BenchmarkCookieHash(b *testing.B) {
	r := rand.New(rand.NewSource(1))
	conns := make([]network.ConnectionStats, 1024)
	for i := range conns {
		conns[i] = randomCookieConn(r)
	}
	h := newCookieHasher()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Hash(&conns[i%len(conns)])
	}
}
