// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

// Package procfs holds procfs related files
package procfs

import (
	"net"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestParseNetTCPv4(t *testing.T) {
	content := `  sl  local_address rem_address   st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 3500007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   991        0 6155 1 0000000000000000 100 0 0 10 5
   1: 3600007F:0035 00000000:0000 0A 00000000:00000000 00:00000000 00000000   991        0 6157 1 0000000000000000 100 0 0 10 5
   2: 0100007F:A227 00000000:0000 0A 00000000:00000000 00:00000000 00000000  1000        0 176859 1 0000000000000000 100 0 0 10 0
   3: 0100007F:CD1E 0100007F:A227 01 00000000:00000000 00:00000000 00000000  1000        0 175959 1 0000000000000000 20 4 20 10 -1
   4: 0100007F:A227 0100007F:CD1E 01 00000000:00000000 00:00000000 00000000  1000        0 176863 1 0000000000000000 20 4 30 10 -1`

	res, err := parseNetIPFromReader(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	expected := []netIPEntry{
		{
			ip:    net.ParseIP("127.0.0.53"),
			port:  53,
			inode: 6155,
		},
		{
			ip:    net.ParseIP("127.0.0.54"),
			port:  53,
			inode: 6157,
		},
		{
			ip:    net.ParseIP("127.0.0.1"),
			port:  41511,
			inode: 176859,
		},
		{
			ip:    net.ParseIP("127.0.0.1"),
			port:  52510,
			inode: 175959,
		},
		{
			ip:    net.ParseIP("127.0.0.1"),
			port:  41511,
			inode: 176863,
		},
	}

	assert.Equal(t, expected, res)
}

func TestParseNetTCPv6(t *testing.T) {
	content := `  sl  local_address                         remote_address                        st tx_queue rx_queue tr tm->when retrnsmt   uid  timeout inode
   0: 00000000000000000000000000000000:0016 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 7254 1 0000000000000000 100 0 0 10 0
   1: 00000000000000000000000000000000:3241 00000000000000000000000000000000:0000 0A 00000000:00000000 00:00000000 00000000     0        0 8529 1 0000000000000000 100 0 0 10 0
   2: 0000000000000000FFFF00000246A8C0:0016 0000000000000000FFFF00000146A8C0:D6DB 01 00000000:00000000 02:000047EC 00000000     0        0 9193 4 0000000000000000 20 4 1 10 79`

	res, err := parseNetIPFromReader(strings.NewReader(content))
	if err != nil {
		t.Fatal(err)
	}

	expected := []netIPEntry{
		{
			ip:    net.ParseIP("::"),
			port:  22,
			inode: 7254,
		},
		{
			ip:    net.ParseIP("::"),
			port:  12865,
			inode: 8529,
		},
		{
			ip:    net.ParseIP("0000:0000:0000:0000:0000:ffff:c0a8:4602"),
			port:  22,
			inode: 9193,
		},
	}

	assert.Equal(t, expected, res)
}
