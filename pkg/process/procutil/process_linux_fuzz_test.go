// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procutil

import (
	"testing"
	"time"
)

func FuzzParseStatContent(f *testing.F) {
	// Real /proc/[pid]/stat seeds
	f.Add([]byte("1234 (bash) S 1233 1234 1234 34816 1234 4194304 1000 0 0 0 10 5 0 0 20 0 1 0 12345 10000000 500 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"))
	f.Add([]byte("42 (Web Content) S 1 42 42 0 -1 4194560 2000 0 0 0 100 50 0 0 20 0 30 0 98765 500000000 10000 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"))
	f.Add([]byte("1 (systemd) S 0 1 1 0 -1 4194560 50000 2000000 100 200 500 300 1000 500 20 0 1 0 1 170000000 3000 18446744073709551615 0 0 0 0 0 0 671173123 4096 1260 0 0 0 17 0 0 0 0 0 0"))
	// Edge cases: parens in name, short content
	f.Add([]byte("999 ((sd-pam)) S 998 999 999 0 -1 1077936448 0 0 0 0 0 0 0 0 20 0 1 0 500 0 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"))
	f.Add([]byte(""))
	f.Add([]byte("no parens here"))

	p := &probe{clockTicks: DefaultClockTicks}

	f.Fuzz(func(t *testing.T, data []byte) {
		sInfo := &statInfo{cpuStat: &CPUTimesStat{}}
		p.parseStatContent(data, sInfo, 1, time.Now())
	})
}

func FuzzParseStatusLine(f *testing.F) {
	f.Add([]byte("Name:\tbash"))
	f.Add([]byte("State:\tS (sleeping)"))
	f.Add([]byte("Uid:\t1000\t1000\t1000\t1000"))
	f.Add([]byte("Gid:\t1000\t1000\t1000\t1000"))
	f.Add([]byte("NSpid:\t1234\t1"))
	f.Add([]byte("Threads:\t4"))
	f.Add([]byte("VmRSS:\t12345 kB"))
	f.Add([]byte("VmSize:\t100000 kB"))
	f.Add([]byte("VmSwap:\t0 kB"))
	f.Add([]byte("voluntary_ctxt_switches:\t100"))
	f.Add([]byte("nonvoluntary_ctxt_switches:\t50"))
	f.Add([]byte(""))

	p := &probe{clockTicks: DefaultClockTicks}

	f.Fuzz(func(t *testing.T, line []byte) {
		sInfo := &statusInfo{
			uids:        []int32{},
			gids:        []int32{},
			memInfo:     &MemoryInfoStat{},
			ctxSwitches: &NumCtxSwitchesStat{},
		}
		p.parseStatusLine(line, sInfo)
	})
}

func FuzzParseIOLine(f *testing.F) {
	f.Add([]byte("syscr: 1234"))
	f.Add([]byte("syscw: 5678"))
	f.Add([]byte("read_bytes: 4096"))
	f.Add([]byte("write_bytes: 8192"))
	f.Add([]byte(""))
	f.Add([]byte("unknown_field: 999"))

	p := &probe{clockTicks: DefaultClockTicks}

	f.Fuzz(func(t *testing.T, line []byte) {
		io := &IOCountersStat{ReadBytes: -1, ReadCount: -1, WriteBytes: -1, WriteCount: -1}
		p.parseIOLine(line, io)
	})
}

func FuzzTrimAndSplitBytes(f *testing.F) {
	f.Add([]byte("/usr/bin/bash\x00--login\x00"))
	f.Add([]byte("\x00\x00/usr/bin/docker\x00-H\x00fd://\x00"))
	f.Add([]byte("single_arg"))
	f.Add([]byte("\x00\x00\x00"))
	f.Add([]byte(""))

	f.Fuzz(func(t *testing.T, data []byte) {
		trimAndSplitBytes(data)
	})
}
