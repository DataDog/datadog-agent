// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Matrix suite — BOTH operator axes UNSET.
//
// This suite covers column 1 (operator key unset) of BOTH the commands matrix
// and the paths matrix. Commands-axis tests and paths-axis tests live together
// here because they share the same operator-both-unset provisioner config.
//
// Each test isolates ONE axis: the non-tested axis is held in a permissive
// state so the outcome is attributable to the axis under test.
//   - For commands-axis tests: backend_paths = [permissiveBackendPath] so any
//     file read from that path is admitted by the paths axis; only backend_commands
//     varies.
//   - For paths-axis tests: backend_commands = [permissiveBackendCommand] so
//     the commands axis always passes; only backend_paths varies.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type parMatrixBothUnsetSuite struct {
	matrixSuite
}

func TestPARRshellMatrixBothUnset(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parMatrixBothUnsetSuite{matrixSuite: matrixSuite{runnerURN: urn}}
	e2e.Run(t, suite,
		e2e.WithProvisioner(parK8sProvisioner(urn, keyB64, rshellOperatorConfig{})),
		e2e.WithStackName("par-rshell-matrix-both-unset"),
	)
}

// ----- Commands axis — operator unset -----

// TestCommands_OperatorUnset_BackendAbsent_Blocks: backend omits allowedCommands.
// Paths axis is permissive, so the only reason to block is the commands axis.
func (s *parMatrixBothUnsetSuite) TestCommands_OperatorUnset_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":      "cat " + testDataFile,
		"allowedPaths": []string{permissiveBackendPath},
		// allowedCommands absent → nil
	})
	assertBlocked(s.T(), result, "commands axis must block when backend commands is nil")
}

// TestCommands_OperatorUnset_BackendEmpty_Blocks: backend allowedCommands = [].
func (s *parMatrixBothUnsetSuite) TestCommands_OperatorUnset_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "commands axis must block when backend commands is []")
}

// TestCommands_OperatorUnset_BackendNonEmpty_Allows: backend admits cat; operator
// unset means pass-through; paths axis permissive → command executes.
func (s *parMatrixBothUnsetSuite) TestCommands_OperatorUnset_BackendNonEmpty_Allows() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertAllowed(s.T(), result, "pass-through must admit cat; got exit=%d stderr=%v",
		rshellExitCode(result), result.Outputs["stderr"])
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}

// ----- Paths axis — operator unset -----

// TestPaths_OperatorUnset_BackendAbsent_Blocks: backend omits allowedPaths.
// Commands axis is permissive, so the only reason to block is the paths axis.
func (s *parMatrixBothUnsetSuite) TestPaths_OperatorUnset_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		// allowedPaths absent → nil
	})
	assertBlocked(s.T(), result, "paths axis must block when backend paths is nil")
}

// TestPaths_OperatorUnset_BackendEmpty_Blocks: backend allowedPaths = [].
func (s *parMatrixBothUnsetSuite) TestPaths_OperatorUnset_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{},
	})
	assertBlocked(s.T(), result, "paths axis must block when backend paths is []")
}

// TestPaths_OperatorUnset_BackendNonEmpty_Allows: backend admits /host/var/log;
// operator unset means pass-through; commands axis permissive → command executes.
func (s *parMatrixBothUnsetSuite) TestPaths_OperatorUnset_BackendNonEmpty_Allows() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertAllowed(s.T(), result, "pass-through must admit path; got exit=%d stderr=%v",
		rshellExitCode(result), result.Outputs["stderr"])
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}
