// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package ebpftest

import (
	"os"
	"testing"

	"github.com/cilium/ebpf/rlimit"

	"github.com/DataDog/datadog-agent/pkg/config/mock"
	"github.com/DataDog/datadog-agent/pkg/ebpf/features"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
	"github.com/DataDog/datadog-agent/pkg/util/funcs"
)

// fentryProbeFunc is the kernel function used to probe for fentry support. It
// must be a stable kernel symbol present across the supported distros.
const fentryProbeFunc = "tcp_close"

var fentrySupported = funcs.MemoizeNoError(func() bool {
	return features.SupportsFentry(fentryProbeFunc) == nil
})

// SupportedBuildModes returns the build modes supported on the current host
func SupportedBuildModes() []BuildMode {
	modes := []BuildMode{RuntimeCompiled, CORE}
	if !prebuilt.IsDeprecated() || os.Getenv("TEST_PREBUILT_OVERRIDE") == "true" {
		modes = append(modes, Prebuilt)
	}
	if fentrySupported() {
		modes = append(modes, Fentry)
	}
	if os.Getenv("TEST_EBPFLESS_OVERRIDE") == "true" {
		modes = append(modes, Ebpfless)
	}

	return modes
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
