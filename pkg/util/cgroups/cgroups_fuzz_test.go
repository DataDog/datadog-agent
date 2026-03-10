// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux

package cgroups

import (
	"testing"
)

// FuzzGetIOStats exercises the full cgroupv2 IO stats parsing pipeline against
// fuzzer-controlled file content. This catches panics in parseV2IOFn which
// reports an error on short lines but lacks a return statement, allowing
// execution to fall through to fields[0] when the line produces zero fields
// (e.g. an empty line).
func FuzzGetIOStats(f *testing.F) {
	// valid io.stat seeds
	f.Add([]byte("259:0 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0\n"))
	f.Add([]byte("8:16 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0\n"))
	f.Add([]byte("259:0 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0\n8:16 rbytes=278528 wbytes=11623899136 rios=6 wios=2744940 dbytes=0 dios=0\n"))
	// valid io.max seed
	f.Add([]byte("8:16 rbps=2097152 wbps=max riops=max wiops=120\n"))
	// crash seed: empty line → strings.Fields("") == []string{} → fields[0] OOB in parseV2IOFn
	f.Add([]byte("\n"))
	f.Add([]byte(""))
	// other edge cases
	f.Add([]byte("notanumber rbytes=0\n"))
	f.Add([]byte("8:16\n"))
	f.Add([]byte("8:16 rbytes=\n"))

	cfs := newCgroupMemoryFS("/test/fs/cgroup")
	cfs.enableControllers("io")
	cg := cfs.createCgroupV2("fuzz", "fuzz-container")

	f.Fuzz(func(t *testing.T, data []byte) {
		cfs.setCgroupV2File(cg, "io.stat", string(data))
		stats := &IOStats{}
		//nolint:errcheck
		cg.GetIOStats(stats)
	})
}

// FuzzParseCPUSetFormat exercises the CPU set string parser. Although it does
// not currently panic, it silently corrupts the count for backwards ranges
// (e.g. "10-5") due to unsigned integer underflow, making it a good coverage
// target to catch future regressions.
func FuzzParseCPUSetFormat(f *testing.F) {
	f.Add("0,1,5-8")
	f.Add("0")
	f.Add("2-3")
	f.Add("")
	// backwards range: uint64 underflow produces a huge count
	f.Add("10-5")
	// non-numeric parts: Atoi errors silently return 0
	f.Add("abc-def")
	f.Add("0-abc")
	f.Add(",,,")
	f.Add("0-0")

	f.Fuzz(func(t *testing.T, line string) {
		ParseCPUSetFormat(line)
	})
}
