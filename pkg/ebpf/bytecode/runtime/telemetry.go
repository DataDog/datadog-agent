// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"time"

	model "github.com/DataDog/agent-payload/v5/process"

	"github.com/DataDog/datadog-agent/pkg/telemetry"
	"github.com/DataDog/datadog-agent/pkg/util/kernel"
	"github.com/DataDog/datadog-agent/pkg/util/log"
)

var rcTelemetry = struct {
	success telemetry.Counter
	error   telemetry.Counter
}{
	success: telemetry.NewCounter("ebpf__runtime_compilation__compile", "success", []string{"platform", "platform_version", "kernel", "arch", "asset", "result"}, "counter of runtime compilation compile successes"),
	error:   telemetry.NewCounter("ebpf__runtime_compilation__compile", "error", []string{"platform", "platform_version", "kernel", "arch", "asset", "result"}, "counter of runtime compilation compile errors"),
}

// CompilationResult enumerates runtime compilation success & failure modes
type CompilationResult int

const (
	notAttempted CompilationResult = iota
	compilationSuccess
	kernelVersionErr
	verificationError
	_
	outputFileErr
	_
	compilationErr
	resultReadErr
	headerFetchErr
	compiledOutputFound
	inputHashError
)

// CompilationTelemetry is telemetry collected per-program when attempting runtime compilation
type CompilationTelemetry struct {
	compilationEnabled  bool
	compilationResult   CompilationResult
	compilationDuration time.Duration
}

func newCompilationTelemetry() CompilationTelemetry {
	return CompilationTelemetry{
		compilationEnabled: false,
		compilationResult:  notAttempted,
	}
}

// CompilationEnabled returns whether runtime compilation has been enabled
func (tm *CompilationTelemetry) CompilationEnabled() bool {
	return tm.compilationEnabled
}

// CompilationResult returns the result of runtime compilation
func (tm *CompilationTelemetry) CompilationResult() int32 {
	return int32(tm.compilationResult)
}

// CompilationDurationNS returns the duration of runtime compilation
func (tm *CompilationTelemetry) CompilationDurationNS() int64 {
	return tm.compilationDuration.Nanoseconds()
}

// SubmitTelemetry sends telemetry using the provided statsd client
func (tm *CompilationTelemetry) SubmitTelemetry(filename string) {
	if !tm.compilationEnabled || tm.compilationResult == notAttempted {
		return
	}

	platform, err := kernel.Platform()
	if err != nil {
		log.Warnf("failed to retrieve host platform information: %s", err)
		return
	}
	platformVersion, err := kernel.PlatformVersion()
	if err != nil {
		log.Warnf("failed to get platform version: %s", err)
		return
	}
	kernelVersion, err := kernel.Release()
	if err != nil {
		log.Warnf("failed to get kernel version: %s", err)
		return
	}
	arch, err := kernel.Machine()
	if err != nil {
		log.Warnf("failed to get kernel architecture: %s", err)
		return
	}

	tags := []string{
		platform,
		platformVersion,
		kernelVersion,
		arch,
		filename,
		model.RuntimeCompilationResult(tm.compilationResult).String(),
	}

	if tm.compilationResult == compilationSuccess || tm.compilationResult == compiledOutputFound {
		rcTelemetry.success.Inc(tags...)
	} else {
		rcTelemetry.error.Inc(tags...)
	}
}
