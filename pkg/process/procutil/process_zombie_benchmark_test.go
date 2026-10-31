// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package procutil

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

const benchmarkProcessCount = 1000

var benchmarkZombieCounts = []int{0, 1, 100, 1000}

func benchmarkProbeForRoot(b *testing.B, root string) *probe {
	b.Helper()
	p := NewProcessProbe(WithProcFSRoot(root), WithPermission(true)).(*probe)
	b.Cleanup(p.Close)
	return p
}

func benchmarkSyntheticProcessSet(b *testing.B, zombies int) (*probe, []int32) {
	b.Helper()

	root := b.TempDir()
	if err := os.WriteFile(filepath.Join(root, "stat"), []byte("btime 1\n"), 0644); err != nil {
		b.Fatal(err)
	}

	pids := make([]int32, 0, benchmarkProcessCount)
	for i := 0; i < benchmarkProcessCount; i++ {
		pid := int32(10000 + i)
		pids = append(pids, pid)
		pidPath := filepath.Join(root, strconv.Itoa(int(pid)))
		if err := os.MkdirAll(filepath.Join(pidPath, "fd"), 0755); err != nil {
			b.Fatal(err)
		}

		state, stateCode := "S (sleeping)", "S"
		if i < zombies {
			state, stateCode = "Z (zombie)", "Z"
		}
		files := map[string]string{
			"status":  "Name:\tbenchmark-process\nState:\t" + state + "\nUid:\t2000\t2000\t2000\t2000\nGid:\t501\t501\t501\t501\nNSpid:\t" + strconv.Itoa(int(pid)) + "\nThreads:\t1\nVmRSS:\t100 kB\nVmSize:\t200 kB\nVmSwap:\t0 kB\nvoluntary_ctxt_switches:\t7\nnonvoluntary_ctxt_switches:\t8\n",
			"stat":    strconv.Itoa(int(pid)) + " (benchmark-process) " + stateCode + " 42 0 0 0 0 0 0 0 0 0 10 5 0 0 20 0 1 0 100 0 0 0 0\n",
			"comm":    "benchmark-process\n",
			"cmdline": "benchmark-process\x00--flag\x00",
			"statm":   "1 2 3 4 5 6 7\n",
			"io":      "syscr: 11\nsyscw: 12\nread_bytes: 13\nwrite_bytes: 14\n",
		}
		for name, contents := range files {
			if err := os.WriteFile(filepath.Join(pidPath, name), []byte(contents), 0644); err != nil {
				b.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(pidPath, "fd", "1"), []byte("sentinel"), 0644); err != nil {
			b.Fatal(err)
		}
		if err := os.Symlink("/tmp", filepath.Join(pidPath, "cwd")); err != nil {
			b.Fatal(err)
		}
		if err := os.Symlink("/bin/true", filepath.Join(pidPath, "exe")); err != nil {
			b.Fatal(err)
		}
	}

	return benchmarkProbeForRoot(b, root), pids
}

func benchmarkSyntheticProcessCollection(b *testing.B, collectProcess bool) {
	for _, zombies := range benchmarkZombieCounts {
		b.Run(fmt.Sprintf("zombies=%d", zombies), func(b *testing.B) {
			p, pids := benchmarkSyntheticProcessSet(b, zombies)
			now := time.Now()
			b.ReportAllocs()
			b.ResetTimer()
			if collectProcess {
				for i := 0; i < b.N; i++ {
					for _, pid := range pids {
						proc, err := p.processFromPID(pid, true, now)
						if err != nil || proc == nil {
							b.Fatalf("pid %d: proc=%v err=%v", pid, proc, err)
						}
					}
				}
				return
			}
			for i := 0; i < b.N; i++ {
				stats, err := p.StatsForPIDs(pids, now)
				if err != nil || len(stats) != len(pids) {
					b.Fatalf("stats=%d pids=%d err=%v", len(stats), len(pids), err)
				}
			}
		})
	}
}

// BenchmarkZombieMixSyntheticProcess measures the per-PID process collection core.
// It excludes PID discovery and result map construction.
func BenchmarkZombieMixSyntheticProcess(b *testing.B) {
	benchmarkSyntheticProcessCollection(b, true)
}

func BenchmarkZombieMixSyntheticStats(b *testing.B) {
	benchmarkSyntheticProcessCollection(b, false)
}
