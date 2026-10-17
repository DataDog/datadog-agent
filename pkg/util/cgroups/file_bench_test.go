// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"strconv"
	"testing"
)

func BenchmarkParsePSI(b *testing.B) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")
	cg := cfs.createCgroupV2("bench-psi", "kubepods/pod1")
	cfs.setCgroupV2File(cg, "cpu.pressure", "some avg10=42.64 avg60=43.72 avg300=25.76 total=114289003\nfull avg10=1.23 avg60=4.56 avg300=7.89 total=987654321")

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var some, full PSIStats
		_ = parsePSI(cfs, cg.pathFor("cpu.pressure"), &some, &full)
	}
}

func BenchmarkParse2ColumnStats(b *testing.B) {
	cfs := newCgroupMemoryFS("/test/fs/cgroup")
	cg := cfs.createCgroupV2("bench-2col", "kubepods/pod1")
	cfs.setCgroupV2File(cg, "memory.stat", sampleCgroupV2MemoryStat)

	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		var total uint64
		_ = parse2ColumnStats(cfs, cg.pathFor("memory.stat"), 0, 1, func(key, value string) error {
			v, err := strconv.ParseUint(value, 10, 64)
			if err == nil {
				total += v
			}
			return nil
		})
	}
}
