// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

// This doesn't need BPF, but it's built with this tag to only run with
// system-probe tests.
//go:build test && linux_bpf

package module

import (
	"net"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

func BenchmarkParseHexIP(b *testing.B) {
	ipv4Hex := []byte("0B01F40A")                               // 10.244.1.11
	ipv6Hex := []byte("B80D0120000000000000000001000000")       // 2001:db8::1
	ipv6MappedHex := []byte("0000000000000000FFFF00000B01F40A") // ::ffff:10.244.1.11

	b.Run("IPv4", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parseHexIPBytes(ipv4Hex, "v4")
		}
	})

	b.Run("IPv6", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parseHexIPBytes(ipv6Hex, "v6")
		}
	})

	b.Run("IPv6MappedIPv4", func(b *testing.B) {
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			parseHexIPBytes(ipv6MappedHex, "v6")
		}
	})
}

func BenchmarkParseEstablishedConnLine(b *testing.B) {
	// Typical line from /proc/net/tcp (ESTABLISHED connection)
	lineV4 := []byte("   0: 0B01F40A:1F90 0C01F40A:B5CA 01 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0")

	// Typical line from /proc/net/tcp6 with IPv6-mapped IPv4 (common in containers)
	lineV6Mapped := []byte("   0: 0000000000000000FFFF00000B01F40A:138D 0000000000000000FFFF00000C01F40A:B5CA 01 00000000:00000000 00:00000000 00000000  1000        0 12345 1 0000000000000000 100 0 0 10 0")

	b.Run("IPv4", func(b *testing.B) {
		listening := make(map[uint64]uint16)
		established := make(map[uint64]*establishedConnInfo)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			processNetTCPLine(lineV4, "v4", listening, established)
		}
	})

	b.Run("IPv6MappedIPv4", func(b *testing.B) {
		listening := make(map[uint64]uint16)
		established := make(map[uint64]*establishedConnInfo)
		b.ReportAllocs()
		for i := 0; i < b.N; i++ {
			processNetTCPLine(lineV6Mapped, "v6", listening, established)
		}
	})
}

func BenchmarkParseNetTCPComplete(b *testing.B) {
	// Create some established connections for the benchmark
	listeners := make([]net.Listener, 0, 10)
	conns := make([]net.Conn, 0, 20)

	for i := 0; i < 10; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(b, err)
		listeners = append(listeners, l)

		// Accept connections in goroutine
		go func(l net.Listener) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				// Keep connection open
				_ = conn
			}
		}(l)

		// Create a client connection
		clientConn, err := net.Dial("tcp", l.Addr().String())
		require.NoError(b, err)
		conns = append(conns, clientConn)
	}

	b.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
		for _, l := range listeners {
			l.Close()
		}
	})

	pid := os.Getpid()

	b.Run("Combined", func(b *testing.B) {
		b.ReportAllocs()
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			parseNetTCPComplete(pid)
		}
	})
}

func BenchmarkGetConnections(b *testing.B) {
	// This benchmark measures the full getConnections loop including namespace caching.
	// Create some listening sockets and established connections.
	listeners := make([]net.Listener, 0, 10)
	conns := make([]net.Conn, 0, 20)

	for i := 0; i < 10; i++ {
		l, err := net.Listen("tcp", "localhost:0")
		require.NoError(b, err)
		listeners = append(listeners, l)

		// Accept connections in goroutine
		go func(l net.Listener) {
			for {
				conn, err := l.Accept()
				if err != nil {
					return
				}
				_ = conn
			}
		}(l)

		// Create a client connection
		clientConn, err := net.Dial("tcp", l.Addr().String())
		require.NoError(b, err)
		conns = append(conns, clientConn)
	}

	b.Cleanup(func() {
		for _, c := range conns {
			c.Close()
		}
		for _, l := range listeners {
			l.Close()
		}
	})

	d := newDiscovery()

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		d.getConnections()
	}
}
