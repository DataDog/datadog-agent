// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package dnfv2

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/rpm/dnfv2/repo"
	"github.com/DataDog/datadog-agent/pkg/util/kernel/headers/download/types"
)

// BenchmarkExtractRPMPackage measures extraction of a real kernel-devel RPM that is
// already in memory, as the RPM backends hold it after download. It is skipped
// unless DD_BENCH_KERNEL_RPM names a package. Run one benchmark per process so
// that peak-RSS-MB is not shared with others:
//
//	dda inv test --targets=./pkg/util/kernel/headers/download/rpm/dnfv2 \
//	  --test-args='-test.run=^$ -test.bench=BenchmarkExtractRPMPackage -test.benchtime=1x -test.count=10' \
//	  --bazel-args='--test_output=all --test_env=DD_BENCH_KERNEL_RPM=/abs/kernel-devel.rpm --test_env=DD_BENCH_KERNEL_RELEASE=<release>'
func BenchmarkExtractRPMPackage(b *testing.B) {
	path := os.Getenv("DD_BENCH_KERNEL_RPM")
	if path == "" {
		b.Skip("DD_BENCH_KERNEL_RPM is not set")
	}
	data, err := os.ReadFile(path)
	require.NoError(b, err)
	pkg := &repo.PkgInfo{Header: repo.PkgInfoHeader{Name: "kernel-devel"}}
	target := &types.Target{Uname: types.Utsname{Kernel: os.Getenv("DD_BENCH_KERNEL_RELEASE")}}

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		require.NoError(b, ExtractPackage(pkg, data, b.TempDir(), target, nil))
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
