// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package network

import (
	"os"
	"testing"
)

// FuzzReadProcNetWithStatus exercises the /proc/net/tcp parser against
// fuzzer-controlled file content. The fieldIterator helpers should be
// panic-free; this harness provides regression coverage and guards against
// divergence between this implementation and the parallel USM parseAddress
// implementation (which previously had a confirmed crash).
func FuzzReadProcNetWithStatus(f *testing.F) {
	// Normal IPv4 tcp seed (header line + data lines)
	f.Add([]byte(`  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 0200007F:B600 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 61632 1 ffff88003cc20780 100 0 0 10 0
   1: 00000000:A160 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16753 1 ffff880034e46d00 100 0 0 10 0
`))
	// Normal IPv6 tcp seed
	f.Add([]byte(`  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:ADA0 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 16755 1 ffff88003b34b180 100 0 0 10 0
`))
	// Empty file
	f.Add([]byte(""))
	f.Add([]byte("\n"))
	// Header only
	f.Add([]byte("  sl  local_address rem_address   st\n"))
	// Line with no colon in local_address
	f.Add([]byte("header\n   0: DEADBEEF 00000000:0000 0A\n"))
	// Line with empty fields
	f.Add([]byte("header\n   0:  :  \n"))
	// State not matching tcpListen
	f.Add([]byte("header\n   0: 0200007F:B600 00000000:0000 01 rest\n"))

	f.Fuzz(func(t *testing.T, data []byte) {
		tmpf, err := os.CreateTemp("", "fuzz-proc-net-*")
		if err != nil {
			t.Fatal(err)
		}
		defer os.Remove(tmpf.Name())
		if _, err := tmpf.Write(data); err != nil {
			tmpf.Close()
			t.Fatal(err)
		}
		tmpf.Close()

		_, _ = readProcNetWithStatus(tmpf.Name(), tcpListen)
	})
}
