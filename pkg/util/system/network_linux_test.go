// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package system

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/testutil"
)

func TestParseProcessRoutes(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-process-routes")
	assert.NoError(t, err)
	defer dummyProcDir.RemoveAll() // clean up

	for _, tc := range []struct {
		pid          int
		routes       string
		destinations []NetworkRoute
	}{
		// two interfaces
		{
			pid: 1245,
			routes: testutil.Detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			destinations: []NetworkRoute{{
				Interface: "eth0",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0x010011AC,
			}, {
				Interface: "eth0",
				Subnet:    0x000011AC,
				Mask:      0x0000FFFF,
				Gateway:   0x0000000,
			}},
		},
		// previous int32 overflow bug, now we parse uint32
		{
			pid: 1249,
			routes: testutil.Detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    FEFEA8C0    0003    0   0   0   00000000    0   0   0
                eth0    00FEA8C0    00000000    0001    0   0   0   00FFFFFF    0   0   0
			`),
			destinations: []NetworkRoute{{
				Interface: "eth0",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0xFEFEA8C0,
			}, {
				Interface: "eth0",
				Subnet:    0x00FEA8C0,
				Mask:      0x00FFFFFF,
				Gateway:   0x00000000,
			}},
		},
		// Multiple interfaces
		{
			pid: 5153,
			routes: testutil.Detab(`
                Iface   Destination Gateway     Flags   RefCnt  Use Metric  Mask        MTU Window  IRTT
                eth0    00000000    010011AC    0003    0   0   0   00000000    0   0   0
                eth0    000011AC    00000000    0001    0   0   0   0000FFFF    0   0   0
                eth1    000012AC    00000000    0001    0   0   0   0000FFFF    0   0   0
            `),
			destinations: []NetworkRoute{{
				Interface: "eth0",
				Subnet:    0x00000000,
				Mask:      0x00000000,
				Gateway:   0x010011AC,
			}, {
				Interface: "eth0",
				Subnet:    0x000011AC,
				Mask:      0x0000FFFF,
				Gateway:   0x00000000,
			}, {
				Interface: "eth1",
				Subnet:    0x000012AC,
				Mask:      0x0000FFFF,
				Gateway:   0x00000000,
			}},
		},
	} {
		t.Run("", func(t *testing.T) {
			// Create temporary files on disk with the routes and stats.
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(tc.pid), "net", "route"), tc.routes)
			assert.NoError(t, err)

			dest, err := ParseProcessRoutes(dummyProcDir.RootPath, tc.pid)
			assert.NoError(t, err)
			assert.Equal(t, tc.destinations, dest)
		})
	}
}

func TestParseProcessIPs(t *testing.T) {
	dummyProcDir, err := testutil.NewTempFolder("test-process-ips")
	assert.NoError(t, err)
	defer dummyProcDir.RemoveAll() // clean up

	exampleNetFibTrieFileContent := `
Main:
  +-- 0.0.0.0/1 2 0 2
     +-- 0.0.0.0/4 2 0 2
        |-- 0.0.0.0
           /0 universe UNICAST
        +-- 10.4.0.0/24 2 1 2
           |-- 10.4.0.0
              /32 link BROADCAST
              /24 link UNICAST
           +-- 10.4.0.192/26 2 0 2
              |-- 10.4.0.216
                 /32 host LOCAL
              |-- 10.4.0.255
                 /32 link BROADCAST
     +-- 127.0.0.0/8 2 0 2
        +-- 127.0.0.0/31 1 0 0
           |-- 127.0.0.0
              /32 link BROADCAST
              /8 host LOCAL
           |-- 127.0.0.1
              /32 host LOCAL
        |-- 127.255.255.255
           /32 link BROADCAST
Local:
  +-- 0.0.0.0/1 2 0 2
     +-- 0.0.0.0/4 2 0 2
        |-- 0.0.0.0
           /0 universe UNICAST
        +-- 10.4.0.0/24 2 1 2
           |-- 10.4.0.0
              /32 link BROADCAST
              /24 link UNICAST
           +-- 10.4.0.192/26 2 0 2
              |-- 10.4.0.216
                 /32 host LOCAL
              |-- 10.4.0.255
                 /32 link BROADCAST
     +-- 127.0.0.0/8 2 0 2
        +-- 127.0.0.0/31 1 0 0
           |-- 127.0.0.0
              /32 link BROADCAST
              /8 host LOCAL
           |-- 127.0.0.1
              /32 host LOCAL
        |-- 127.255.255.255
           /32 link BROADCAST
`

	tests := []struct {
		name                      string
		pid                       int
		procNetFibTrieFileContent string
		filterFunc                func(string) bool
		expectedIPs               []string
	}{
		{
			name:                      "standard case",
			pid:                       123,
			procNetFibTrieFileContent: exampleNetFibTrieFileContent,
			expectedIPs:               []string{"10.4.0.216", "127.0.0.1"},
		},
		{
			name:                      "with filter",
			pid:                       123,
			procNetFibTrieFileContent: exampleNetFibTrieFileContent,
			filterFunc:                func(ip string) bool { return ip != "127.0.0.1" },
			expectedIPs:               []string{"10.4.0.216"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			// Create temporary "fib_trie" file on disk
			err = dummyProcDir.Add(filepath.Join(strconv.Itoa(test.pid), "net", "fib_trie"), test.procNetFibTrieFileContent)
			assert.NoError(t, err)

			resultIPs, err := ParseProcessIPs(dummyProcDir.RootPath, test.pid, test.filterFunc)

			assert.NoError(t, err)
			assert.ElementsMatch(t, test.expectedIPs, resultIPs)
		})
	}
}

func TestGetProcessNetDevInode(t *testing.T) {
	fakeProc := t.TempDir()
	// Test setup: pid 2 has same network namespace as pid 1, pid 3 different
	require.NoError(t, os.MkdirAll(filepath.Join(fakeProc, "1", "net"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(fakeProc, "2", "net"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(fakeProc, "3", "net"), 0o755))

	require.NoError(t, os.WriteFile(filepath.Join(fakeProc, "1", "net", "dev"), []byte{}, 0o644))
	require.NoError(t, os.Link(filepath.Join(fakeProc, "1", "net", "dev"), filepath.Join(fakeProc, "2", "net", "dev")))
	require.NoError(t, os.WriteFile(filepath.Join(fakeProc, "3", "net", "dev"), []byte{}, 0o644))

	pid2inode, err := GetProcessNetDevInode(fakeProc, 2)
	if assert.NoError(t, err) {

		pid2HostNet := IsProcessHostNetwork(fakeProc, pid2inode)
		if assert.NotNil(t, pid2HostNet) {
			assert.True(t, *pid2HostNet)
		}
	}

	pid3inode, err := GetProcessNetDevInode(fakeProc, 3)
	if assert.NoError(t, err) {

		pid3HostNet := IsProcessHostNetwork(fakeProc, pid3inode)
		if assert.NotNil(t, pid3HostNet) {
			assert.False(t, *pid3HostNet)
		}
	}
}
