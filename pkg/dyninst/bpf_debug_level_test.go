// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux && bpf

package dyninst_test

import (
	"os"
	"strconv"
	"testing"
)

// bpfDebugLevelEnv overrides the eBPF LOG() verbosity requested by tests that
// load the debug object.
const bpfDebugLevelEnv = "DYNINST_BPF_DEBUG_LEVEL"

// defaultBPFDebugLevel is the LOG() level tests ask for by default.
//
// Every LOG(N, ...) at or below the level is live code the verifier has to
// walk, and debug_level is a volatile const the verifier reads from frozen
// .rodata, so the rest is pruned. Asking for level 100 makes all ~148 of
// them live, which pushes probe_run_with_cookie over the kernel's 1M
// instruction verification budget on Linux 7.0 arm64 and makes the program
// unloadable. Level 1 keeps the lines the tests actually read - notably
// "probe_run: continuation aborted at seq=%d" - and costs far less.
//
// Raise it with DYNINST_BPF_DEBUG_LEVEL when debugging by hand, on a kernel
// where the bigger program still verifies.
const defaultBPFDebugLevel = 1

// bpfDebugLevel returns the eBPF LOG() level to load the debug object with.
func bpfDebugLevel(t *testing.T) int {
	s := os.Getenv(bpfDebugLevelEnv)
	if s == "" {
		return defaultBPFDebugLevel
	}
	level, err := strconv.Atoi(s)
	if err != nil {
		t.Fatalf("invalid %s=%q: %v", bpfDebugLevelEnv, s, err)
	}
	return level
}
