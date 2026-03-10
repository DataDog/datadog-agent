// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procutil

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"go.uber.org/atomic"
)

// newFuzzProbe returns a minimal probe suitable for unit-level fuzz tests that
// exercise individual parsing functions. It initialises only the fields those
// functions actually read so that calls like p.bootTime.Load() don't panic.
func newFuzzProbe() *probe {
	return &probe{clockTicks: DefaultClockTicks, bootTime: atomic.NewUint64(0)}
}

func FuzzParseStatContent(f *testing.F) {
	// Real /proc/[pid]/stat seeds
	f.Add([]byte("1234 (bash) S 1233 1234 1234 34816 1234 4194304 1000 0 0 0 10 5 0 0 20 0 1 0 12345 10000000 500 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"))
	f.Add([]byte("42 (Web Content) S 1 42 42 0 -1 4194560 2000 0 0 0 100 50 0 0 20 0 30 0 98765 500000000 10000 18446744073709551615 0 0 0 0 0 0 0 0 0 0 0 0 17 3 0 0 0 0 0"))
	f.Add([]byte("1 (systemd) S 0 1 1 0 -1 4194560 50000 2000000 100 200 500 300 1000 500 20 0 1 0 1 170000000 3000 18446744073709551615 0 0 0 0 0 0 671173123 4096 1260 0 0 0 17 0 0 0 0 0 0"))
	// Edge cases: parens in name, short content
	f.Add([]byte("999 ((sd-pam)) S 998 999 999 0 -1 1077936448 0 0 0 0 0 0 0 0 20 0 1 0 500 0 0 0 0 0 0 0 0 0 0 0 0 0 17 0 0 0 0 0 0"))
	f.Add([]byte(""))
	f.Add([]byte("no parens here"))

	p := newFuzzProbe()

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

	p := newFuzzProbe()

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

	p := newFuzzProbe()

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

// procStatContent is a minimal /proc/stat with a valid btime line so
// bootTime() succeeds during probe initialisation.
const procStatContent = "cpu  0 0 0 0 0 0 0 0 0 0\nbtime 1000000000\n"

// FuzzProcessesByPID exercises the full ProcessesByPID pipeline against a
// fuzzer-controlled fake /proc directory, surfacing crashes that the narrow
// per-function fuzzers cannot reach (e.g. out-of-bounds in parseStatm,
// empty-value slice in parseStatusKV).
func FuzzProcessesByPID(f *testing.F) {
	// seed: valid content from resources/test_procfs/proc/3/
	validStat := []byte("3 (ksoftirqd/0) S 2 0 0 0 -1 69238848 0 0 0 0 2828 0 0 0 20 0 1 0 18 0 0 18446744073709551615 0 0 0 0 0 0 0 2147483647 0 1 0 0 17 0 0 0 0 0 0 0 0 0 0 0 0 0 0\n")
	validStatm := []byte("0 0 0 0 0 0 0\n")
	validStatus := []byte("Name:\tksoftirqd/0\nState:\tS (sleeping)\nPid:\t3\nPPid:\t2\nUid:\t0\t0\t0\t0\nGid:\t0\t0\t0\t0\nThreads:\t1\nvoluntary_ctxt_switches:\t2123987\nnonvoluntary_ctxt_switches:\t23\n")
	validCmdline := []byte("")
	validComm := []byte("ksoftirqd/0\n")

	f.Add(validStat, validStatm, validStatus, validCmdline, validComm)
	// seed: empty statm triggers index-out-of-bounds in parseStatm
	f.Add(validStat, []byte{}, validStatus, validCmdline, validComm)
	// seed: State line with empty value triggers slice-out-of-bounds in parseStatusKV
	f.Add(validStat, validStatm, []byte("Name:\tbash\nState:\t\nPid:\t42\n"), validCmdline, validComm)

	f.Fuzz(func(t *testing.T, stat, statm, status, cmdline, comm []byte) {
		dir := t.TempDir()

		// Write the system-level /proc/stat so bootTime() succeeds.
		if err := os.WriteFile(filepath.Join(dir, "stat"), []byte(procStatContent), 0644); err != nil {
			t.Fatal(err)
		}

		// Create a single fake PID directory.
		pidDir := filepath.Join(dir, "42")
		if err := os.MkdirAll(pidDir, 0755); err != nil {
			t.Fatal(err)
		}
		for name, content := range map[string][]byte{
			"stat":    stat,
			"statm":   statm,
			"status":  status,
			"cmdline": cmdline,
			"comm":    comm,
		} {
			if err := os.WriteFile(filepath.Join(pidDir, name), content, 0644); err != nil {
				t.Fatal(err)
			}
		}

		probe := NewProcessProbe(
			WithProcFSRoot(dir),
			WithPermission(false),
		)
		defer probe.Close()

		//nolint:errcheck
		probe.ProcessesByPID(time.Now(), true)
	})
}
