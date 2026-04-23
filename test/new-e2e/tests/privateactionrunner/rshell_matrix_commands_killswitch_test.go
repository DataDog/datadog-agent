// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Matrix suite — commands axis only. operator.commands = [] (the kill-switch);
// operator.paths = unset (permissive, so the paths axis never drives the outcome).
//
// Covers column 2 of the commands truth matrix:
//
//	| Backend commands ↓ | Effective (expected) |
//	| absent             | ∅                    |
//	| empty              | ∅                    |
//	| non-empty          | ∅                    |
//
// Pre-fix (Bug #1): transform.go treats `[]` YAML as "unset", so the operator
// kill-switch is silently disabled and the backend passes through.
// Post-fix: all three cases block.

package privateactionrunner

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type parMatrixCommandsKillSwitchSuite struct {
	matrixSuite
}

func TestPARRshellMatrixCommandsKillSwitch(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parMatrixCommandsKillSwitchSuite{matrixSuite: matrixSuite{runnerURN: urn}}
	cfg := rshellOperatorConfig{
		commandsSet: true, commands: []string{}, // kill-switch on commands
		// pathsSet: false → operator.paths unset (permissive pass-through)
	}
	e2e.Run(t, suite,
		e2e.WithProvisioner(parK8sProvisioner(urn, keyB64, cfg)),
		e2e.WithStackName("par-rshell-matrix-commands-killswitch"),
	)
}

// TestCommands_OperatorEmpty_BackendAbsent_Blocks: backend omits allowedCommands.
// Outcome blocks either way (nil backend is authoritative), but we assert through
// the commands axis with the paths axis permissive.
func (s *parMatrixCommandsKillSwitchSuite) TestCommands_OperatorEmpty_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":      "cat " + testDataFile,
		"allowedPaths": []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "kill-switch + nil backend commands → ∅")
}

// TestCommands_OperatorEmpty_BackendEmpty_Blocks: both sides [].
func (s *parMatrixCommandsKillSwitchSuite) TestCommands_OperatorEmpty_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "kill-switch + [] backend commands → ∅")
}

// TestCommands_OperatorEmpty_BackendNonEmpty_Blocks: the interesting cell for
// the kill-switch semantics — backend admits cat, operator = [] must override.
// Pre-fix this test FAILS because bug #1 passes the backend through unchanged.
func (s *parMatrixCommandsKillSwitchSuite) TestCommands_OperatorEmpty_BackendNonEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "kill-switch must override non-empty backend commands; got exit=%d stdout=%v",
		rshellExitCode(result), result.Outputs["stdout"])
}
