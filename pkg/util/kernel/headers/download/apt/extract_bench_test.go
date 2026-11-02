// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package apt

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// BenchmarkExtractDebPackage measures extraction of a real linux-headers .deb that
// has already been downloaded to disk, as the apt backend does. It is skipped
// unless DD_BENCH_KERNEL_DEB names a package. Run one benchmark per process so
// that peak-RSS-MB is not shared with others:
//
//	dda inv test --targets=./pkg/util/kernel/headers/download/apt \
//	  --test-args='-test.run=^$ -test.bench=BenchmarkExtractDebPackage -test.benchtime=1x -test.count=10' \
//	  --bazel-args='--test_output=all --test_env=DD_BENCH_KERNEL_DEB=/abs/linux-headers.deb'
func BenchmarkExtractDebPackage(b *testing.B) {
	path := os.Getenv("DD_BENCH_KERNEL_DEB")
	if path == "" {
		b.Skip("DD_BENCH_KERNEL_DEB is not set")
	}
	backend := &Backend{logger: quietLogger{}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, backend.extractPackage(path, b.TempDir()))
	}
	b.StopTimer()
	b.ReportMetric(peakRSSMB(b), "peak-RSS-MB")
}

// peakRSSMB returns the process high-water mark of resident memory (VmHWM).
func peakRSSMB(b *testing.B) float64 {
	f, err := os.Open("/proc/self/status")
	require.NoError(b, err)
	defer f.Close()
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if value, ok := strings.CutPrefix(scanner.Text(), "VmHWM:"); ok {
			kb, err := strconv.ParseFloat(strings.TrimSuffix(strings.TrimSpace(value), " kB"), 64)
			require.NoError(b, err)
			return kb / 1024
		}
	}
	b.Fatal("VmHWM not found in /proc/self/status")
	return 0
}
