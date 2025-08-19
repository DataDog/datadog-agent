// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package procnet

import (
	"net/netip"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestProcNetParsing(t *testing.T) {
	// The example below contains the following connections:
	//
	// 0: 1.1.1.1:8080->1.1.1.1:59836
	// 1: 1.1.1.1:59836->1.1.1.1:8080
	// 2: 10.211.55.6:57434->34.233.170.129:80
	// 3: 1.1.1.1:8080 (Listening Socket)
	procNetFile := createTempFile(t, `sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
  0: 01010101:1F90 01010101:E9BC 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1
  1: 01010101:E9BC 01010101:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 348600 1 0000000000000000 20 0 0 10 -1
  2: 0637D30A:E05A 81AAE922:0050 06 00000000:00000000 03:00001721 00000000     0        0 0 3 0000000000000000
  3: 01010101:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 543187 1 0000000000000000 100 0 0 10 0
`)

	scanner, err := newScanner(procNetFile)
	assert.NoError(t, err)
	defer scanner.Close()

	// Entry 0
	entry, ok := scanner.Next()
	assert.True(t, ok)
	laddr, lport := entry.LocalAddress()
	assert.Equal(t, "1.1.1.1", laddr.String())
	assert.Equal(t, uint16(8080), lport)
	raddr, rport := entry.RemoteAddress()
	assert.Equal(t, "1.1.1.1", raddr.String())
	assert.Equal(t, uint16(59836), rport)
	assert.Equal(t, 347102, entry.Inode())

	// Entry 1
	entry, ok = scanner.Next()
	assert.True(t, ok)
	laddr, lport = entry.LocalAddress()
	assert.Equal(t, "1.1.1.1", laddr.String())
	assert.Equal(t, uint16(59836), lport)
	raddr, rport = entry.RemoteAddress()
	assert.Equal(t, "1.1.1.1", raddr.String())
	assert.Equal(t, uint16(8080), rport)
	assert.Equal(t, 348600, entry.Inode())

	// Entry 2
	entry, ok = scanner.Next()
	assert.True(t, ok)
	laddr, lport = entry.LocalAddress()
	assert.Equal(t, "10.211.55.6", laddr.String())
	assert.Equal(t, uint16(57434), lport)
	raddr, rport = entry.RemoteAddress()
	assert.Equal(t, "34.233.170.129", raddr.String())
	assert.Equal(t, uint16(80), rport)
	assert.Equal(t, 0, entry.Inode())

	// Entry 3
	entry, ok = scanner.Next()
	assert.True(t, ok)
	laddr, lport = entry.LocalAddress()
	assert.Equal(t, "1.1.1.1", laddr.String())
	assert.Equal(t, uint16(8080), lport)
	raddr, rport = entry.RemoteAddress()
	assert.Equal(t, "0.0.0.0", raddr.String())
	assert.Equal(t, uint16(0), rport)
	assert.Equal(t, 543187, entry.Inode())

	_, ok = scanner.Next()
	assert.False(t, ok)
}

func TestProcNetParsingIPv6(t *testing.T) {
	// The example below contains the following connections:
	//
	// 0: [::1]:8080 (Listening Socket)
	// 1: [::1]:8080 <- [::1]:46062
	procNetFile := createTempFile(t, `sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000001000000:1F90 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 542516 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000001000000:1F90 00000000000000000000000001000000:B3EE 06 00000000:00000000 03:000015AA 00000000     0        0 0 3 0000000000000000
`)

	scanner, err := newScanner(procNetFile)
	assert.NoError(t, err)
	defer scanner.Close()

	// Entry 0
	entry, ok := scanner.Next()
	assert.True(t, ok)
	laddr, lport := entry.LocalAddress()
	assert.Equal(t, "::1", laddr.String())
	assert.Equal(t, uint16(8080), lport)
	raddr, rport := entry.RemoteAddress()
	assert.Equal(t, "::", raddr.String())
	assert.Equal(t, uint16(0), rport)
	assert.Equal(t, 542516, entry.Inode())

	// Entry 1
	entry, ok = scanner.Next()
	assert.True(t, ok)
	laddr, lport = entry.LocalAddress()
	assert.Equal(t, "::1", laddr.String())
	assert.Equal(t, uint16(8080), lport)
	raddr, rport = entry.RemoteAddress()
	assert.Equal(t, "::1", raddr.String())
	assert.Equal(t, uint16(46062), rport)
	assert.Equal(t, 0, entry.Inode())

	_, ok = scanner.Next()
	assert.False(t, ok)
}

func BenchmarkScanner(b *testing.B) {
	procNetContents := `sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 01010101:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 347101 1 0000000000000000 100 0 0 10 0
   1: 3500007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   102        0 19914 1 0000000000000000 100 0 0 10 5
   2: 00000000:0016 00000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 21564 1 0000000000000000 100 0 0 10 0
   3: 01010101:1F90 01010101:E9BC 01 00000000:00000000 00:00000000 00000000  1000        0 347102 1 0000000000000000 20 0 0 10 -1
   4: 01010101:E9BC 01010101:1F90 01 00000000:00000000 00:00000000 00000000  1000        0 348600 1 0000000000000000 20 0 0 10 -1
   5: 0637D30A:0016 0237D30A:E5A8 01 00000000:00000000 02:000A8D2D 00000000     0        0 349974 2 0000000000000000 20 9 27 10 -1
   6: 0637D30A:E05A 81AAE922:0050 06 00000000:00000000 03:00001721 00000000     0        0 0 3 0000000000000000
   7: 0637D30A:0016 0237D30A:E5E9 01 00000000:00000000 02:000AF78A 00000000     0        0 350034 4 0000000000000000 20 4 31 10 -1
   8: 0637D30A:0016 0237D30A:E2A7 01 00000000:00000000 02:0004DAA7 00000000     0        0 296943 2 0000000000000000 20 4 30 7 7
   9: 0637D30A:0016 0237D30A:C3B3 01 00000000:00000000 02:0001DAA7 00000000     0        0 215210 2 0000000000000000 20 4 30 10 60
  10: 0637D30A:0016 0237D30A:E446 01 00000000:00000000 02:0007BB1F 00000000     0        0 348601 2 0000000000000000 20 4 25 10 -1
  11: 0637D30A:DDE8 31C26597:01BB 01 00000000:00000000 00:00000000 00000000   112        0 154366 1 0000000000000000 61 4 32 10 -1
`
	f, _ := os.CreateTemp(b.TempDir(), "")
	f.WriteString(procNetContents)
	filePath := f.Name()
	f.Close()

	scanner, _ := newScanner(filePath)
	defer scanner.Close()
	b.ReportAllocs()
	b.ResetTimer()

	// The purpose of this benchmark is to ensure there are no allocations when *scanning*
	// the file. Note that when we reach the end of the file we're rewinding to offset 0
	// to ensure that the Go benchmarking will collect enough samples.
	for i := 0; i < b.N; i++ {
		_, ok := scanner.Next()
		if !ok {
			scanner.file.Seek(0, 0)
		}
	}
}

func BenchmarkAddressParsing(b *testing.B) {
	const (
		expectedIP   = "1.1.1.1"
		expectedPort = uint16(8080)
	)

	// Local Address (01010101:1F90) represents "1.1.1.1:8080"
	procNetEntry := []byte("0: 01010101:1F90 00000000:0000 0A 00000000:00000000 00:00000000 00000000 1000" +
		"0 347101 1 0000000000000000 100 0 0 10 0")

	buffer := make([]byte, 16)
	entry := newEntry(procNetEntry, buffer)
	b.ReportAllocs()
	b.ResetTimer()

	var (
		laddr netip.Addr
		lport uint16
	)

	for i := 0; i < b.N; i++ {
		laddr, lport = entry.LocalAddress()
	}
	b.StopTimer()

	// Use previous results from the last method calls to ensure they won't be optimized away
	if laddr.String() != expectedIP || lport != expectedPort {
		b.Fatalf("benchmark didn't produced the expected values. expected=%s:%d got=%s:%d",
			expectedIP,
			expectedPort,
			laddr.String(),
			lport,
		)
	}
}

func BenchmarkInodeParsing(b *testing.B) {
	// The inode of the entry below is 347102
	procNetEntry := []byte("0: 01010101:1F90 01010101:E9BC 01 00000000:00000000 00:00000000 00000000 1000 0 347102 1 0000000000000000 20 0 0 10 -1")
	const expectedInode = 347102

	buffer := make([]byte, 16)
	entry := newEntry(procNetEntry, buffer)
	b.ReportAllocs()
	b.ResetTimer()

	var inode int
	for i := 0; i < b.N; i++ {
		inode = entry.Inode()
	}
	b.StopTimer()

	// Use previous results from the last method calls to ensure they won't be optimized away
	if inode != expectedInode {
		b.Fatalf("benchmark didn't produced the expected values. expected=%d got=%d",
			expectedInode,
			inode,
		)
	}
}

func createTempFile(t *testing.T, contents string) string {
	f, err := os.CreateTemp(t.TempDir(), "")
	assert.NoError(t, err)
	defer f.Close()
	_, err = f.WriteString(contents)
	assert.NoError(t, err)
	return f.Name()
}
