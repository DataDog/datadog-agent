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
// Deliberately does NOT include ebpftest.Fentry. Most callers of this helper
// live in pkg/network/usm and exercise a USM monitor without ever constructing
// a connection tracer. Since config.EnableFentry is only ever read by the
// fentry connection tracer, a "fentry" pass over those tests loads usm.o as
// CO-RE and re-runs the CO-RE pass verbatim — no fentry code is executed. That
// duplicate pass is not free: it added ~18min per arch and pushed
// pkg/network/usm past its 55m per-package test timeout in KMT.
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
// when the host is eligible for it.
//
// Only for tests that construct a connection tracer, which is what makes the
// fentry pass meaningful — see the note on SupportedBuildModes.
//
// Fentry eligibility is delegated to ebpftest rather than duplicated here: the
// gate (kernel floor / deadlock-symbol boundary / TEST_FENTRY_OVERRIDE) is
// subtle and has regressed before when copied. Note this selects the fentry
// *connection tracer* while usm.o still loads as CO-RE — usm.o has no fentry
// variant and needs none.
func SupportedBuildModesWithFentry() []ebpftest.BuildMode {
	modes := SupportedBuildModes()
	if slices.Contains(ebpftest.SupportedBuildModes(), ebpftest.Fentry) {
		modes = append(modes, ebpftest.Fentry)
	}

	return modes
}
