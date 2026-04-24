// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Matrix suite — paths axis only. operator.paths = [] (the kill-switch);
// operator.commands = unset (permissive).
//
// Covers column 2 of the paths truth matrix. Mirrors commands-killswitch on the
// other axis: the commands axis is pass-through (operator unset + backend
// admits cat), so all blocking must come from the paths axis.

package privateactionrunner

import (
	"testing"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type parMatrixPathsKillSwitchSuite struct {
	matrixSuite
}

func TestPARRshellMatrixPathsKillSwitch(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parMatrixPathsKillSwitchSuite{matrixSuite: matrixSuite{runnerURN: urn}}
	cfg := rshellOperatorConfig{
		pathsSet: true, paths: []string{}, // kill-switch on paths
		// commandsSet: false → operator.commands unset (permissive)
	}
	e2e.Run(t, suite,
		e2e.WithProvisioner(parK8sProvisioner(urn, keyB64, cfg)),
		e2e.WithStackName("par-rshell-matrix-paths-killswitch"),
	)
}

// TestPaths_OperatorEmpty_BackendAbsent_Blocks: backend omits allowedPaths.
func (s *parMatrixPathsKillSwitchSuite) TestPaths_OperatorEmpty_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
	})
	assertBlocked(s.T(), result, "kill-switch + nil backend paths → ∅")
}

// TestPaths_OperatorEmpty_BackendEmpty_Blocks: both sides [].
func (s *parMatrixPathsKillSwitchSuite) TestPaths_OperatorEmpty_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{},
	})
	assertBlocked(s.T(), result, "kill-switch + [] backend paths → ∅")
}

// TestPaths_OperatorEmpty_BackendNonEmpty_Blocks: the interesting cell. Backend
// admits /host/var/log; operator = [] must override. Pre-fix: Bug #1 fails this.
func (s *parMatrixPathsKillSwitchSuite) TestPaths_OperatorEmpty_BackendNonEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "kill-switch must override non-empty backend paths; got exit=%d stdout=%v",
		rshellExitCode(result), result.Outputs["stdout"])
}
