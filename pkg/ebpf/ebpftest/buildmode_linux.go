// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

package ebpftest

import (
	"os"
	"runtime"
	"testing"

	"github.com/DataDog/datadog-agent/pkg/util/kernel"
)

var hostPlatform string
var kv = kernel.MustHostVersion()

func init() {
	hostPlatform, _ = kernel.Platform()
}

// SupportedBuildModes returns the build modes supported on the current host
func SupportedBuildModes() []BuildMode {
	modes := []BuildMode{Prebuilt, RuntimeCompiled, CORE}
	if os.Getenv("TEST_FENTRY_OVERRIDE") == "true" ||
		(runtime.GOARCH == "amd64" && (hostPlatform == "amazon" || hostPlatform == "amzn") && kv.Major() == 5 && kv.Minor() == 10) {
		modes = append(modes, Fentry)
	}
	return modes
}

// TestBuildModes runs the test under all the provided build modes
func TestBuildModes(t *testing.T, modes []BuildMode, name string, fn func(t *testing.T)) {
	for _, mode := range modes {
		TestBuildMode(t, mode, name, fn)
	}
}

// TestBuildMode runs the test under the provided build mode
func TestBuildMode(t *testing.T, mode BuildMode, name string, fn func(t *testing.T)) {
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
