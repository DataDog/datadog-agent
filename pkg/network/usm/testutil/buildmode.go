// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2024-present Datadog, Inc.

//go:build linux_bpf && test

package testutil

import (
	"os"

	"github.com/DataDog/datadog-agent/pkg/ebpf/ebpftest"
	"github.com/DataDog/datadog-agent/pkg/ebpf/prebuilt"
)

// SupportedBuildModes returns the build modes supported on the current host
func SupportedBuildModes() []ebpftest.BuildMode {
	modes := []ebpftest.BuildMode{ebpftest.RuntimeCompiled, ebpftest.CORE}
	if !prebuilt.IsDeprecated() || os.Getenv("TEST_PREBUILT_OVERRIDE") == "true" {
		modes = append(modes, ebpftest.Prebuilt)
	}

	return modes
}
