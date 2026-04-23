// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

// Matrix suite — paths axis only. operator.paths = ["/host/var/log"];
// operator.commands = unset (permissive).
//
// Covers truth-matrix columns 3 (non-empty, disjoint-from-backend) and 4
// (non-empty, overlapping-backend) for the paths axis:
//
//	|                | col 3: Disjoint | col 4: Overlap |
//	| Backend absent | ∅               | ∅ (vacuous)    |
//	| Backend empty  | ∅               | ∅ (vacuous)    |
//	| Backend set    | ∅               | intersection   |

package privateactionrunner

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/DataDog/datadog-agent/test/e2e-framework/testing/e2e"
)

type parMatrixPathsNarrowSuite struct {
	matrixSuite
}

func TestPARRshellMatrixPathsNarrow(t *testing.T) {
	t.Parallel()
	urn, keyB64 := generateTestRunnerIdentity(t)
	suite := &parMatrixPathsNarrowSuite{matrixSuite: matrixSuite{runnerURN: urn}}
	cfg := rshellOperatorConfig{
		pathsSet: true, paths: []string{permissiveBackendPath},
		// operator.commands unset (permissive)
	}
	e2e.Run(t, suite,
		e2e.WithProvisioner(parK8sProvisioner(urn, keyB64, cfg)),
		e2e.WithStackName("par-rshell-matrix-paths-narrow"),
	)
}

// ----- Column 3: operator non-empty, disjoint from backend -----

func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyDisjoint_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
	})
	assertBlocked(s.T(), result, "nil backend → ∅")
}

func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyDisjoint_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{},
	})
	assertBlocked(s.T(), result, "[] backend → ∅")
}

func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyDisjoint_BackendNonEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{"/tmp", "/etc"}, // disjoint from ["/host/var/log"]
	})
	assertBlocked(s.T(), result, "non-empty backend disjoint from operator → empty intersection → ∅")
}

// ----- Column 4: operator non-empty, overlapping backend -----

// Vacuous cell: "overlap with nil" has no meaning, but the Confluence grid lists it.
func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyOverlap_BackendAbsent_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
	})
	assertBlocked(s.T(), result, "nil backend → ∅ regardless of how operator is labelled")
}

// Vacuous cell mirror for backend = [].
func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyOverlap_BackendEmpty_Blocks() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{},
	})
	assertBlocked(s.T(), result, "[] backend → ∅ regardless of how operator is labelled")
}

func (s *parMatrixPathsNarrowSuite) TestPaths_OperatorNonEmptyOverlap_BackendNonEmpty_Allows() {
	result := s.enqueueAndWait(map[string]any{
		"command":         "cat " + testDataFile,
		"allowedCommands": []string{permissiveBackendCommand},
		"allowedPaths":    []string{permissiveBackendPath, "/tmp"},
	})
	assertAllowed(s.T(), result, "overlap on /host/var/log must admit; got exit=%d stderr=%v",
		rshellExitCode(result), result.Outputs["stderr"])
	assert.Contains(s.T(), result.Outputs["stdout"], testDataContent)
}
