// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"os"
	"testing"

	"github.com/cilium/ebpf/rlimit"

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
		if name != "" {
			t.Run(name, fn)
		} else {
			fn(t)
		}
	})
}
