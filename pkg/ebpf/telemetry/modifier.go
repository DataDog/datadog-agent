// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package telemetry

import (
	"io"
	"os"

	manager "github.com/DataDog/ebpf-manager"
)

// ErrorsTelemetryModifier is a modifier that sets up the manager to handle eBPF telemetry.
type ErrorsTelemetryModifier struct {
	SkipProgram func(string) bool
}

// String returns the name of the modifier.
func (t *ErrorsTelemetryModifier) String() string {
	return "ErrorsTelemetryModifier"
}

// BeforeInit sets up the manager to handle eBPF telemetry.
// It will patch the instructions of all the manager probes and `undefinedProbes` provided.
// Constants are replaced for map error and helper error keys with their respective values.
func (t *ErrorsTelemetryModifier) BeforeInit(m *manager.Manager, opts *manager.Options, bytecode io.ReaderAt) error {
	// TODO: remove this once errors collector is separated from the tracer
	if errorsTelemetry == nil {
		initEBPFTelemetry(os.Getenv("DD_SYSTEM_PROBE_BPF_DIR"))
	}
	return setupForTelemetry(m, opts, errorsTelemetry, bytecode, t.SkipProgram)
}

// AfterInit pre-populates the telemetry maps with entries corresponding to the ebpf program of the manager.
func (t *ErrorsTelemetryModifier) AfterInit(_ *manager.Manager, _ *manager.Options, _ io.ReaderAt) error {
	return nil
}
