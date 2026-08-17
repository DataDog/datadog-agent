// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux && bpf && test

package testutil

import (
	"os"
	"slices"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
)

// SupportedBuildModes returns the build modes supported on the current host.
//
// Does not include ebpftest.Fentry because callers of this helper live in
// pkg/network/usm and exercise a USM monitor without constructing a connection tracer.
//
// Tests that do build a connection tracer want SupportedBuildModesWithFentry.
func SupportedBuildModes() []ebpftest.BuildMode {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() || os.Getenv("TEST_PREBUILT_OVERRIDE") == "true" {
		modes = append(modes, ebpftest.Prebuilt)
	}

	return modes
}

// SupportedBuildModesWithFentry returns SupportedBuildModes plus ebpftest.Fentry
// when the host is eligible for it. Only for tests that construct a connection tracer.
func SupportedBuildModesWithFentry() []ebpftest.BuildMode {
	modes := SupportedBuildModes()
	if slices.Contains(ebpftest.SupportedBuildModes(), ebpftest.Fentry) {
		modes = append(modes, ebpftest.Fentry)
	}

	return modes
}
