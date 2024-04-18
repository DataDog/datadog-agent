// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procnet

import (
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/DataDog/datadog-agent/pkg/network/protocols/http/testutil"
	"github.com/stretchr/testify/assert"
)

func TestGetTCPConnections(t *testing.T) {
	const srvAddress = "127.0.0.1:8080"

	// Create TCP server
	srv := testutil.NewTCPServer(srvAddress, func(c net.Conn) { time.Sleep(time.Minute) }, false)
	done := make(chan struct{})
	srv.Run(done)
	t.Cleanup(func() { close(done) })

	// Connect to it
	clientConn, err := net.Dial("tcp4", srvAddress)
	assert.NoError(t, err)

	// Now let's "manually" obtain the FD and PID of this connection so we can
	// use it later in our test assertions
	connPID := uint32(os.Getpid())
	rawConn, err := clientConn.(*net.TCPConn).SyscallConn()
	assert.NoError(t, err)
	var connFD uint32
	rawConn.Control(func(fd uintptr) {
		connFD = uint32(fd)
	})

	matches := func(c TCPConnection, expected net.Conn) bool {
		return c.FD == connFD &&
			c.PID == connPID &&
			fmt.Sprintf("%s:%d", c.Laddr, c.Lport) == expected.LocalAddr().String() &&
			fmt.Sprintf("%s:%d", c.Raddr, c.Rport) == expected.RemoteAddr().String()
	}

	// Finally, fetch all connections by calling GetTCPConnections() and
	// ensure we can see the connection we created with the proper addresses,
	// and process information (PID & FD)
	connections := GetTCPConnections()
	assert.Condition(t, func() bool {
		for _, c := range connections {
			if matches(c, clientConn) {
				return true
			}
		}
		return false
	})
}

// This benchmark is mostly intended to be executed as a source of pprof data:
// go test -tags=linux_bpf -bench=BenchmarkGetTCPConnections -benchmem -cpuprofile cpu.prof -memprofile mem.prof
func BenchmarkGetTCPConnections(b *testing.B) {
	b.ReportAllocs()

	for i := 0; i < b.N; i++ {
		GetTCPConnections()
	}
}
