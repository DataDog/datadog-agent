// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package runtime

import (
	"errors"
	"fmt"
	"strings"
	"time"

	model "github.com/DataDog/agent-payload/v5/process"
	"github.com/DataDog/datadog-go/v5/statsd"
	"github.com/DataDog/nikos/types"

	"github.com/DataDog/datadog-agent/pkg/metadata/host"
	"github.com/DataDog/datadog-agent/pkg/util/log"
	"github.com/DataDog/datadog-agent/pkg/version"
)

// CompilationResult enumerates runtime compilation success & failure modes
type CompilationResult int

const (
	notAttempted CompilationResult = iota
	compilationSuccess
	kernelVersionErr
	verificationError
	outputDirErr
	outputFileErr
	newCompilerErr // nolint:deadcode,unused
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

// CompilationEnabled returns whether or not runtime compilation has been enabled
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
func (tm *CompilationTelemetry) SubmitTelemetry(filename string, statsdClient statsd.ClientInterface) {
	if !tm.compilationEnabled {
		return
	}

	var platform string
	if target, err := types.NewTarget(); err == nil {
		// Prefer platform information from nikos over platform info from the host package, since this
		// is what kernel header downloading uses
		platform = strings.ToLower(target.Distro.Display)
	} else {
		log.Warnf("failed to retrieve host platform information from nikos: %s", err)
		platform = host.GetStatusInformation().Platform
	}

	tags := []string{
		fmt.Sprintf("asset:%s", filename),
		fmt.Sprintf("agent_version:%s", version.AgentVersion),
		fmt.Sprintf("platform:%s", platform),
	}

	if tm.compilationResult != notAttempted {
		var resultTag string
		if tm.compilationResult == compilationSuccess || tm.compilationResult == compiledOutputFound {
			resultTag = "success"
		} else {
			resultTag = "failure"
		}

		rcTags := append(tags,
			fmt.Sprintf("result:%s", resultTag),
			fmt.Sprintf("reason:%s", model.RuntimeCompilationResult(tm.compilationResult).String()),
		)

		if err := statsdClient.Count("datadog.system_probe.runtime_compilation.attempted", 1.0, rcTags, 1.0); err != nil && !errors.Is(err, statsd.ErrNoClient) {
			log.Warnf("error submitting runtime compilation metric to statsd: %s", err)
		}
	}
}
