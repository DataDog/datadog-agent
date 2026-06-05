// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"os"
	"runtime"
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
// using per-arch kernel-version floors as a stand-in for features.SupportsFentry()
//
// TODO: replace this hardcoded kernel-version gate with a call to
// features.SupportsFentry() once the pkg/ebpf import-cycle reorg lands (see
// PRs #51821/#51825). Importing pkg/ebpf/features here today pulls in an
// amd64-only test import cycle (ebpftest -> features -> kernelbugs ->
// bytecode/runtime -> pkg/ebpf), so until that's untangled we approximate
// fentry support with per-arch kernel-version floors. This broadens KMT
// coverage beyond the old amazon/5.10-only carveout to every KMT distro with
// a new-enough kernel. fentry/fexit BPF trampolines: amd64 since 5.5, arm64
// since 6.0. All current KMT images at/above these floors ship BTF.
func fentrySupported() bool {
	switch runtime.GOARCH {
	case "amd64":
		return kv >= kernel.VersionCode(5, 5, 0)
	case "arm64":
		return kv >= kernel.VersionCode(6, 0, 0)
	default:
		return false
	}
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
