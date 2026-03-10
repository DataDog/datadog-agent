// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procnet

import (
	"testing"
)

// FuzzNewEntry exercises the /proc/net/tcp line parser and all entry accessor
// methods. It catches a panic in parseAddress: when the address field has a
// colon but an empty port (e.g. "AABBCCDD:"), hex.Decode returns n=0 with no
// error, and the subsequent binary.BigEndian.Uint16(buffer[:0]) panics because
// the slice has fewer than 2 bytes.
func FuzzNewEntry(f *testing.F) {
	// valid IPv4 entry line
	f.Add([]byte("  0: 01010101:1F90 01010101:E9BC 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1\n"))
	// valid IPv6 entry line
	f.Add([]byte("  0: 00000000000000000000000001000000:1F90 00000000000000000000000001000000:E9BC 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1\n"))
	// empty line
	f.Add([]byte(""))
	// crash seed: laddr with empty port → hex.Decode returns n=0 → binary.BigEndian.Uint16(buf[:0]) panics
	f.Add([]byte("  0: AABBCCDD: 01010101:E9BC 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1\n"))
	// raddr with empty port
	f.Add([]byte("  0: 01010101:1F90 AABBCCDD: 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1\n"))
	// address with only a colon
	f.Add([]byte("  0: : : 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1\n"))

	buf := make([]byte, 16)
	f.Fuzz(func(t *testing.T, line []byte) {
		e := newEntry(line, buf)
		_, _ = e.LocalAddress()
		_, _ = e.RemoteAddress()
		_ = e.ConnectionState()
		_ = e.Inode()
	})
}
