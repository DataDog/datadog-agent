// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2025-present Datadog, Inc.

//go:build linux_bpf

package procscan

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata"
	model "github.com/DataDog/datadog-agent/pkg/discovery/tracermetadata/model"
	"github.com/DataDog/datadog-agent/pkg/dyninst/process"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

// benchProcessDelays and benchScanInterval mirror the values the agent runs
// with, so that a measured scan is the same amount of work as a production
// tick and can be expressed as a fraction of a core.
var benchProcessDelays = []time.Duration{
	3 * time.Second,
	100 * time.Second,
	1000 * time.Second,
}

const benchScanInterval = 3 * time.Second

// benchProcessCounts and benchFDCounts describe the hosts we care about: a
// small host, a busy host, and a container host with thousands of processes,
// each with either a modest or a large number of open file descriptors.
var (
	benchProcessCounts = []int{100, 500, 2000}
	benchFDCounts      = []int{16, 128}
)

// BenchmarkScanSteadyState measures one scan tick on a host where nothing has
// changed since the previous tick.
func BenchmarkScanSteadyState(b *testing.B) {
	forEachBenchSize(b, func(b *testing.B, tree syntheticProcfs) {
		s := NewScanner(tree.root, benchProcessDelays...)
		// The first scan examines every pre-existing process, which is a
		// different workload; see BenchmarkScanFirstScan.
		_, _, err := s.Scan()
		require.NoError(b, err)

		b.ReportAllocs()
		for b.Loop() {
			discovered, removed, err := s.Scan()
			if err != nil {
				b.Fatal(err)
			}
			if len(discovered) != 0 || len(removed) != 0 {
				b.Fatalf(
					"process set changed mid-benchmark: %d new, %d removed",
					len(discovered), len(removed),
				)
			}
		}
		reportPctCore(b)
	})
}

// BenchmarkScanFirstScan measures the burst after an agent restart, when every
// process that is already running is examined in a single tick.
func BenchmarkScanFirstScan(b *testing.B) {
	forEachBenchSize(b, func(b *testing.B, tree syntheticProcfs) {
		b.ReportAllocs()
		for b.Loop() {
			s := NewScanner(tree.root, benchProcessDelays...)
			if _, _, err := s.Scan(); err != nil {
				b.Fatal(err)
			}
		}
		reportPctCore(b)
	})
}

// BenchmarkStartTimeRead calibrates the synthetic tree against the real thing.
// Stat files in the synthetic tree are ordinary tmpfs files, while the kernel
// formats /proc/<pid>/stat on every read, so the synthetic numbers above are
// optimistic by whatever factor this benchmark reports.
func BenchmarkStartTimeRead(b *testing.B) {
	tree := makeSyntheticProcfs(b, 1, 16)
	for _, tc := range []struct {
		name string
		root string
		pid  int32
	}{
		{name: "real_procfs", root: "/proc", pid: int32(os.Getpid())},
		{name: "synthetic", root: tree.root, pid: int32(tree.pids[0])},
	} {
		b.Run(tc.name, func(b *testing.B) {
			reader := newStartTimeReader(tc.root)
			startTime, err := reader.read(tc.pid)
			require.NoError(b, err)
			require.NotZero(b, startTime)

			b.ReportAllocs()
			for b.Loop() {
				if _, err := reader.read(tc.pid); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// BenchmarkTracerMetadataMiss calibrates the descriptor walk that a process
// without tracer metadata pays for. The fds metric is the number of
// descriptors walked, which is what the cost scales with.
func BenchmarkTracerMetadataMiss(b *testing.B) {
	type benchCase struct {
		name string
		root string
		pid  int
	}
	cases := []benchCase{
		{name: "real_procfs_self", root: "/proc", pid: os.Getpid()},
	}
	for _, fds := range benchFDCounts {
		tree := makeSyntheticProcfs(b, 1, fds)
		cases = append(cases, benchCase{
			name: fmt.Sprintf("synthetic_fds=%d", fds),
			root: tree.root,
			pid:  int(tree.pids[0]),
		})
	}
	for _, tc := range cases {
		b.Run(tc.name, func(b *testing.B) {
			fdDir := filepath.Join(tc.root, strconv.Itoa(tc.pid), "fd")
			entries, err := os.ReadDir(fdDir)
			require.NoError(b, err)
			require.NotEmpty(b, entries)
			// A lookup that fails to open the descriptor directory returns the
			// same error without walking anything, so the miss must be
			// confirmed against a directory we know is readable.
			_, err = tracermetadata.GetTracerMetadata(tc.pid, tc.root)
			require.ErrorIs(b, err, kernel.ErrMemFdFileNotFound)

			b.ReportAllocs()
			for b.Loop() {
				_, _ = tracermetadata.GetTracerMetadata(tc.pid, tc.root)
			}
			b.ReportMetric(float64(len(entries)), "fds")
		})
	}
}

// TestSyntheticProcfsScanWork pins down which per-process work a scan of the
// synthetic tree performs, so that the benchmarks cannot silently measure a
// scan that skipped everything.
func TestSyntheticProcfsScanWork(t *testing.T) {
	const overflow = 76
	for _, tc := range []struct {
		name     string
		numProcs int
		// steadyStateReads is how many start times a scan after the first one
		// still has to read from disk, which is one per process that did not
		// fit in the start time cache.
		steadyStateReads int
	}{
		{name: "cache_fits", numProcs: 64, steadyStateReads: 0},
		{
			name:             "cache_overflows",
			numProcs:         defaultStartTimeCacheSize + overflow,
			steadyStateReads: overflow,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree := makeSyntheticProcfs(t, tc.numProcs, 8)
			s := NewScanner(tree.root, benchProcessDelays...)

			var startTimeReads, metadataReads int
			readStartTime, readMetadata := s.readStartTime, s.tracerMetadataReader
			s.readStartTime = func(pid int32) (ticks, error) {
				startTimeReads++
				return readStartTime(pid)
			}
			s.tracerMetadataReader = func(pid int32) (model.TracerMetadata, error) {
				metadataReads++
				return readMetadata(pid)
			}

			discovered, removed, err := s.Scan()
			require.NoError(t, err)
			require.Empty(t, discovered)
			require.Empty(t, removed)
			require.Equal(t, tc.numProcs, startTimeReads,
				"first scan reads the start time of every process")
			require.Equal(t, tc.numProcs, metadataReads,
				"first scan looks for tracer metadata in every process")

			startTimeReads, metadataReads = 0, 0
			discovered, removed, err = s.Scan()
			require.NoError(t, err)
			require.Empty(t, discovered)
			require.Empty(t, removed)
			require.Equal(t, tc.steadyStateReads, startTimeReads)
			require.Zero(t, metadataReads,
				"every process is older than the longest window")
		})
	}
}

// syntheticProcfs is a procfs-shaped tree that the scanner can be pointed at,
// so scan cost can be measured against a chosen process count instead of
// whatever happens to be running on the host.
type syntheticProcfs struct {
	root       string
	pids       []uint32
	startTimes []ticks
	fdsPerProc int
}

// makeSyntheticProcfs builds a tree of numProcs processes with fdsPerProc open
// descriptors each. No process carries tracer metadata: that is both the
// common case on a host and the expensive one, because the lookup only gives
// up after walking every descriptor.
func makeSyntheticProcfs(
	tb testing.TB, numProcs, fdsPerProc int,
) syntheticProcfs {
	now, err := nowTicks()
	require.NoError(tb, err)

	// Every process must be older than the longest discovery window. A younger
	// one would age into a window partway through a benchmark and turn a
	// steady-state tick into a discovery tick.
	const minAge = ticks(1010 * clkTck)
	const ageSpacing = ticks(7)
	ageSpread := ageSpacing * ticks(numProcs)
	if now < minAge+ageSpread {
		tb.Skipf("host uptime of %d ticks is too short for %d processes",
			now, numProcs)
	}

	systemExes, selfExe := benchExeTargets(tb)
	tree := syntheticProcfs{
		root:       tb.TempDir(),
		pids:       make([]uint32, 0, numProcs),
		startTimes: make([]ticks, 0, numProcs),
		fdsPerProc: fdsPerProc,
	}
	const firstPID = 1000
	for i := range numProcs {
		pid := uint32(firstPID + i)
		startTime := now - minAge - ageSpacing*ticks(i)
		tree.pids = append(tree.pids, pid)
		tree.startTimes = append(tree.startTimes, startTime)

		procDir := filepath.Join(tree.root, strconv.FormatUint(uint64(pid), 10))
		fdDir := filepath.Join(procDir, "fd")
		require.NoError(tb, os.MkdirAll(fdDir, 0o755))
		require.NoError(tb, os.WriteFile(
			filepath.Join(procDir, "stat"),
			[]byte(syntheticProcStat(pid, startTime)),
			0o644,
		))
		for fd := range fdsPerProc {
			require.NoError(tb, os.Symlink(
				"/dev/null", filepath.Join(fdDir, strconv.Itoa(fd)),
			))
		}
		// A single shared executable would make any per-executable caching
		// look free, so spread the processes over several binaries. One in
		// twenty gets a Go binary.
		exe := systemExes[i%len(systemExes)]
		if i%20 == 0 {
			exe = selfExe
		}
		require.NoError(tb, os.Symlink(exe, filepath.Join(procDir, "exe")))
	}

	tree.verify(tb)
	return tree
}

// verify asserts that the tree exercises the paths a scan takes: stat files
// that parse, an executable that resolves, and a descriptor walk that
// completes and finds no tracer metadata.
func (tree syntheticProcfs) verify(tb testing.TB) {
	reader := newStartTimeReader(tree.root)
	for _, i := range []int{0, len(tree.pids) - 1} {
		pid := tree.pids[i]
		startTime, err := reader.read(int32(pid))
		require.NoError(tb, err)
		require.Equal(tb, uint64(tree.startTimes[i]), startTime)

		_, err = process.ResolveExecutable(tree.root, int32(pid))
		require.NoError(tb, err)

		fdDir := filepath.Join(tree.root, strconv.Itoa(int(pid)), "fd")
		entries, err := os.ReadDir(fdDir)
		require.NoError(tb, err)
		require.Len(tb, entries, tree.fdsPerProc)

		_, err = tracermetadata.GetTracerMetadata(int(pid), tree.root)
		require.ErrorIs(tb, err, kernel.ErrMemFdFileNotFound)
	}
}

// syntheticProcStat renders a /proc/<pid>/stat line whose 22nd field is the
// given start time.
func syntheticProcStat(pid uint32, startTime ticks) string {
	fields := []string{
		strconv.FormatUint(uint64(pid), 10),
		"(bench-proc)",
		"S",
	}
	// Fields 4 through 21 are not read; only their count matters.
	for i := 4; i <= 21; i++ {
		fields = append(fields, strconv.Itoa(i*7))
	}
	fields = append(fields, strconv.FormatUint(uint64(startTime), 10))
	fields = append(fields, "0", "0", "0")
	return strings.Join(fields, " ") + "\n"
}

// benchExeTargets returns real, readable binaries to point the fake exe links
// at, since resolving an executable opens and stats the target.
func benchExeTargets(tb testing.TB) (systemExes []string, selfExe string) {
	for _, candidate := range []string{
		"/bin/sh", "/bin/cat", "/bin/ls", "/usr/bin/env", "/bin/true",
	} {
		f, err := os.Open(candidate)
		if err != nil {
			continue
		}
		f.Close()
		systemExes = append(systemExes, candidate)
	}
	if len(systemExes) == 0 {
		tb.Skip("no readable system binary found for fake exe links")
	}
	selfExe, err := os.Executable()
	require.NoError(tb, err)
	return systemExes, selfExe
}

// forEachBenchSize runs fn against a fresh tree for every host size.
func forEachBenchSize(
	b *testing.B, fn func(b *testing.B, tree syntheticProcfs),
) {
	for _, numProcs := range benchProcessCounts {
		for _, fdsPerProc := range benchFDCounts {
			name := fmt.Sprintf("procs=%d/fds=%d", numProcs, fdsPerProc)
			b.Run(name, func(b *testing.B) {
				fn(b, makeSyntheticProcfs(b, numProcs, fdsPerProc))
			})
		}
	}
}

// reportPctCore reports the measured per-scan cost as a percentage of one core
// at the production scan interval.
func reportPctCore(b *testing.B) {
	nsPerScan := float64(b.Elapsed().Nanoseconds()) / float64(b.N)
	pct := nsPerScan / float64(benchScanInterval.Nanoseconds()) * 100
	b.ReportMetric(pct, "pct_core_at_3s")
}
