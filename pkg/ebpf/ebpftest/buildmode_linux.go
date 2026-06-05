// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"os"
	"testing"

	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var kv = kernel.MustHostVersion()

// SupportedBuildModes returns the build modes supported on the current host
func SupportedBuildModes() []BuildMode {
	modes := []BuildMode{RuntimeCompiled, CORE}
	if !prebuilt.IsDeprecated() || os.Getenv("TEST_PREBUILT_OVERRIDE") == "true" {
		modes = append(modes, Prebuilt)
	}
	if os.Getenv("TEST_FENTRY_OVERRIDE") == "true" || fentrySupported() {
		modes = append(modes, Fentry)
	}
	if os.Getenv("TEST_EBPFLESS_OVERRIDE") == "true" {
		modes = append(modes, Ebpfless)
	}

	return modes
}

// fentrySupported approximates whether the fentry tracer can load on this host,
// using a kernel-version floor as a stand-in for features.SupportsFentry().
//
// The binding constraint is NOT the fentry/fexit trampoline floor (amd64 5.5,
// arm64 6.0) but the RCU-exit deadlock bug: fentry.LoadTracer refuses to load on
// any kernel carrying the tasks_rcu_exit_srcu symbol (present ~5.7 through 6.8 on
// both arches), which was removed by the upstream fix in 6.9. So fentry only
// actually loads on kernels >= 6.9 (modulo distro backports). Since 6.9 is above
// both trampoline floors, we gate on it for every arch. This keeps the fentry
// build mode from being selected on kernels where fentry.LoadTracer would fail
// its HasTasksRCUExitLockSymbol() check (e.g. KMT amazon_2023/6.1,
// ubuntu_24.04/6.8) — which previously turned the whole fentry matrix red.
//
// This is an upstream-version proxy: the deadlock symbol's presence depends on
// the distro patch level, not the upstream version, so a kernel that calls
// itself < 6.9 but has the fix backported (e.g. Ubuntu 6.8.0-86, where the
// symbol is gone) actually runs fentry fine but is excluded here. Use
// TEST_FENTRY_OVERRIDE=true to force the fentry build mode on such kernels —
// fentry.LoadTracer's runtime HasTasksRCUExitLockSymbol() check still gates the
// real load, so the override is safe (it only loads where the symbol is absent).
//
// TODO: replace this hardcoded gate with features.SupportsFentry() once the
// pkg/ebpf import-cycle reorg lands (see PRs #51821/#51825). Importing
// pkg/ebpf/features here today pulls in an amd64-only test import cycle
// (ebpftest -> features -> kernelbugs -> bytecode/runtime -> pkg/ebpf). Only the
// real SupportsFentry() probe gets backported sub-6.9 kernels right.
func fentrySupported() bool {
	return kv >= kernel.VersionCode(6, 9, 0)
}

// TestBuildModes runs the test under all the provided build modes
func TestBuildModes(t *testing.T, modes []BuildMode, name string, fn func(t *testing.T)) {
	for _, mode := range modes {
		TestBuildMode(t, mode, name, fn)
	}
}

var removeMemlock = funcs.MemoizeNoError(rlimit.RemoveMemlock)

// TestBuildMode runs the test under the provided build mode
func TestBuildMode(t *testing.T, mode BuildMode, name string, fn func(t *testing.T)) {
	if err := removeMemlock(); err != nil {
		t.Fatal(err)
	}
	t.Run(mode.String(), func(t *testing.T) {
		for k, v := range mode.Env() {
			t.Setenv(k, v)
		}
		_ = mock.NewSystemProbe(t)
		if name != "" {
			t.Run(name, fn)
		} else {
			fn(t)
		}
	})
}
