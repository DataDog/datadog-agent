// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Matrix suite — commands axis only. operator.commands = ["rshell:cat"];
// operator.paths = unset (permissive).
//
// Covers truth-matrix columns 3 (non-empty, disjoint-from-backend) and 4
// (non-empty, overlapping-backend) for the commands axis. The operator
// configuration is fixed at non-empty; tests vary the backend to hit each
// (row, column) cell:
//
//	|                | col 3: Disjoint | col 4: Overlap |
//	| Backend absent | ∅               | ∅ (vacuous)    |
//	| Backend empty  | ∅               | ∅ (vacuous)    |
//	| Backend set    | ∅               | intersection   |
//
// For the "vacuous" cells (backend nil/[] × operator.overlap), the "overlap"
// semantics can't hold — there's nothing in the backend to overlap with. The
// tests still exist to match the Confluence 12-cell grid one-for-one; their
// bodies are identical to their disjoint counterparts on the same row.

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type parMatrixCommandsNarrowSuite struct {
	matrixSuite
}

func TestPARRshellMatrixCommandsNarrow(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parMatrixCommandsNarrowSuite{matrixSuite: matrixSuite{runnerURN: urn}}
	cfg := rshellOperatorConfig{
		commandsSet: true, commands: []string{"rshell:cat"},
		// operator.paths unset (permissive)
	}
	e2e.Run(t, suite,
		e2e.WithProvisioner(parK8sProvisioner(urn, keyB64, cfg)),
		e2e.WithStackName("par-rshell-matrix-commands-narrow"),
	)
}

// ----- Column 3: operator non-empty, disjoint from backend -----

func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyDisjoint_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":      "cat " + testDataFile,
		"allowedPaths": []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "nil backend → ∅")
}

func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyDisjoint_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "[] backend → ∅")
}

// Backend non-empty AND disjoint from operator ["rshell:cat"]: intersection empty.
func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyDisjoint_BackendNonEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{"rshell:ls", "rshell:find"}, // no rshell:cat
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "non-empty backend disjoint from operator → empty intersection → ∅")
}

// ----- Column 4: operator non-empty, overlapping backend -----

// Vacuous cell: "overlap with nil" has no meaning, but the Confluence grid lists
// it. The test body is identical to its disjoint counterpart on the same row.
func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyOverlap_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":      "cat " + testDataFile,
		"allowedPaths": []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "nil backend → ∅ regardless of how operator is labelled")
}

// Vacuous cell mirror for backend = [].
func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyOverlap_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{},
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertBlocked(s.T(), result, "[] backend → ∅ regardless of how operator is labelled")
}

// Backend non-empty AND overlapping operator ["rshell:cat"]: intersection admits cat.
func (s *parMatrixCommandsNarrowSuite) TestCommands_OperatorNonEmptyOverlap_BackendNonEmpty_Allows() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{"rshell:cat", "rshell:ls"}, // overlaps operator on rshell:cat
		"allowedPaths":    []string{permissiveBackendPath},
	})
	assertAllowed(s.T(), result, "overlap on cat must admit; got exit=%d stderr=%v",
		rshellExitCode(result), result.Outputs["stderr"])
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}
