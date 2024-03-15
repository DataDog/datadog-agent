// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import "testing"

// SupportedBuildModes returns the build modes supported on the current host
func SupportedBuildModes() []BuildMode {
	return []BuildMode{Prebuilt}
}

// TestBuildModes runs the test under all the provided build modes
func TestBuildModes(t *testing.T, modes []BuildMode, name string, fn func(t *testing.T)) { //nolint:revive // TODO fix revive unused-parameter
	// ignore provided modes and only use prebuilt
	TestBuildMode(t, Prebuilt, name, fn)
}

// TestBuildMode runs the test under the provided build mode
func TestBuildMode(t *testing.T, mode BuildMode, name string, fn func(t *testing.T)) {
	if mode != Prebuilt {
		t.Skipf("unsupported build mode %s", mode)
		return
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
