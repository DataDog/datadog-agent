// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2016-present Datadog, Inc.

//go:build linux_bpf

package module

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/DataDog/datadog-agent/pkg/dyninst/ir"
)

// The synthetic runtime.recovery probe is internal — its failures must
// not surface to the user via diagnostics.reportError. attachedProgram-
// Impl.ReportProbeError gates on probe kind and drops the event for the
// recovery probe; user-defined probes go through as ExecutionFailed.
func TestReportProbeError_FiltersRuntimeRecoveryProbe(t *testing.T) {
	fu := &fakeDiagsUploader{}
	ap := &attachedProgramImpl{
		runtime: &runtimeImpl{
			diagnostics: newDiagnosticsManager(fu),
		},
		runtimeID: procRuntimeID{service: "svc", runtimeID: "rid"},
	}

	recoveryProbe := recoveryProbeStub{}
	ap.ReportProbeError(recoveryProbe, errors.New("recovery probe tripped"))
	require.Equal(t, 0, fu.len(),
		"recovery probe diagnostic must not reach the uploader")

	userProbe := userProbeStub{}
	ap.ReportProbeError(userProbe, errors.New("user probe tripped"))
	require.Equal(t, 1, fu.len(),
		"user probe diagnostic must reach the uploader")
}

// recoveryProbeStub implements ir.ProbeDefinition with the same
// (ID, Kind) the production runtime.recovery probe carries.
type recoveryProbeStub struct {
	testProbeDefinition
}

func (recoveryProbeStub) GetID() string         { return ir.RuntimeRecoveryProbeID }
func (recoveryProbeStub) GetVersion() int       { return 0 }
func (recoveryProbeStub) GetKind() ir.ProbeKind { return ir.ProbeKindRuntimeRecovery }

// userProbeStub stands in for a user-configured probe — any non-recovery
// kind is fine; the filter only cares about ProbeKindRuntimeRecovery.
type userProbeStub struct {
	testProbeDefinition
}

func (userProbeStub) GetID() string         { return "user" }
func (userProbeStub) GetVersion() int       { return 1 }
func (userProbeStub) GetKind() ir.ProbeKind { return ir.ProbeKindLog }
