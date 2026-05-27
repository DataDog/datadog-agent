// Unless explicitly stated otherwise all files in this repository are licensed
// under the Apache License Version 2.0.
// This product includes software developed at Datadog (https://www.datadoghq.com/).
// Copyright 2026-present Datadog, Inc.

package interfaceresolver

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// auditScenarioCandidates: same shape as vlanColocatedCandidates but
// repeated here to keep the audit test file self-contained.
func auditScenarioCandidates() []Candidate {
	return []Candidate{
		{Index: 1, Name: "Gi0/1", MAC: "aa:bb:cc:dd:ee:01", Type: IfTypeEthernetCsmacd},
		{Index: 101, Name: "Gi0/1.100", MAC: "aa:bb:cc:dd:ee:01", Type: IfTypeL2Vlan},
		{Index: 102, Name: "Gi0/1.200", MAC: "aa:bb:cc:dd:ee:01", Type: IfTypeL2Vlan},
		{Index: 2, Name: "Gi0/2", MAC: "aa:bb:cc:dd:ee:02", Type: IfTypeEthernetCsmacd},
	}
}

func TestAuditReport_RateHelpers_NoSample(t *testing.T) {
	rep := AuditReport{}
	assert.Equal(t, 0.0, rep.MisResolutionRate())
	assert.Equal(t, 0.0, rep.CoverageRate())
}

func TestAuditReport_RateHelpers_Basic(t *testing.T) {
	rep := AuditReport{
		Sample:              100,
		MatchedCorrect:      80,
		MatchedIncorrect:    2,
		UnresolvedAmbiguous: 15,
		Unmatched:           3,
	}
	// 2 / (80+2) = ~0.0244
	assert.InDelta(t, 2.0/82.0, rep.MisResolutionRate(), 1e-6)
	// (80+2) / 100 = 0.82
	assert.InDelta(t, 0.82, rep.CoverageRate(), 1e-6)
}

func TestAudit_CorrectResolutions(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	oracle := []OracleEntry{
		// Unique MAC → match.
		{MAC: "aa:bb:cc:dd:ee:02", IfIndex: 2},
		// Ambiguous MAC + correct expected-type hint → physical.
		{MAC: "aa:bb:cc:dd:ee:01", Hint: ContextHint{ExpectedType: IfTypeEthernetCsmacd}, IfIndex: 1},
		// Ambiguous MAC + correct name hint → sub-interface .100.
		{MAC: "aa:bb:cc:dd:ee:01", Hint: ContextHint{VLANID: 100, NameHint: "Gi0/1.100"}, IfIndex: 101},
	}

	rep := a.Run(oracle)
	assert.Equal(t, 3, rep.Sample)
	assert.Equal(t, 3, rep.MatchedCorrect)
	assert.Equal(t, 0, rep.MatchedIncorrect)
	assert.Equal(t, 0, rep.UnresolvedAmbiguous)
	assert.Equal(t, 0, rep.Unmatched)
	assert.InDelta(t, 0.0, rep.MisResolutionRate(), 1e-9)
	assert.InDelta(t, 1.0, rep.CoverageRate(), 1e-9)
}

func TestAudit_MisResolutionDetected(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	oracle := []OracleEntry{
		// Oracle says: physical port (ifIndex 1), but we ask for the
		// sub-interface; the resolver will incorrectly pick the
		// virtual one. This is the kind of mis-resolution the audit
		// must catch.
		{
			MAC:     "aa:bb:cc:dd:ee:01",
			Hint:    ContextHint{VLANID: 100, NameHint: "Gi0/1.100"},
			IfIndex: 1, // oracle disagrees with the resolver
		},
	}
	rep := a.Run(oracle)

	assert.Equal(t, 1, rep.Sample)
	assert.Equal(t, 0, rep.MatchedCorrect)
	assert.Equal(t, 1, rep.MatchedIncorrect)
	assert.InDelta(t, 1.0, rep.MisResolutionRate(), 1e-9)
}

func TestAudit_UnresolvedAmbiguousIsNotMisResolution(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	oracle := []OracleEntry{
		// Ambiguous MAC, no hint at all → resolver returns
		// UnresolvedAmbiguous. Per PRD, that's the safe path; it
		// must NOT be counted as a mis-resolution.
		{MAC: "aa:bb:cc:dd:ee:01", IfIndex: 1},
	}
	rep := a.Run(oracle)

	assert.Equal(t, 1, rep.Sample)
	assert.Equal(t, 0, rep.MatchedCorrect)
	assert.Equal(t, 0, rep.MatchedIncorrect)
	assert.Equal(t, 1, rep.UnresolvedAmbiguous)
	assert.InDelta(t, 0.0, rep.MisResolutionRate(), 1e-9)
	// Coverage is 0 — resolved/sample = 0/1.
	assert.InDelta(t, 0.0, rep.CoverageRate(), 1e-9)
}

func TestAudit_UnmatchedSplit(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	oracle := []OracleEntry{
		// Oracle agrees no interface → correct unmatched.
		{MAC: "ff:ff:ff:ff:ff:ff", IfIndex: 0},
		// Oracle disagrees → coverage miss (not correctness).
		{MAC: "ee:ee:ee:ee:ee:ee", IfIndex: 99},
	}
	rep := a.Run(oracle)
	assert.Equal(t, 2, rep.Sample)
	assert.Equal(t, 1, rep.MatchedCorrect, "correct unmatched folds into MatchedCorrect")
	assert.Equal(t, 0, rep.MatchedIncorrect)
	assert.Equal(t, 1, rep.Unmatched)
}

// Oracle says "this MAC should map to nothing" but resolver finds an
// interface anyway → that's a mis-resolution (a false positive).
func TestAudit_FalsePositiveCountedAsMisResolution(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	oracle := []OracleEntry{
		{MAC: "aa:bb:cc:dd:ee:02", IfIndex: 0}, // oracle says no, resolver says ifIndex 2
	}
	rep := a.Run(oracle)
	assert.Equal(t, 0, rep.MatchedCorrect)
	assert.Equal(t, 1, rep.MatchedIncorrect)
}

func TestAudit_AggregatedSampleIsRealistic(t *testing.T) {
	r := NewResolver(auditScenarioCandidates())
	a := NewAuditor(r)

	var oracle []OracleEntry
	// 50 correct lookups for the unique MAC.
	for i := 0; i < 50; i++ {
		oracle = append(oracle, OracleEntry{MAC: "aa:bb:cc:dd:ee:02", IfIndex: 2})
	}
	// 30 correct ambiguous lookups (physical port).
	for i := 0; i < 30; i++ {
		oracle = append(oracle, OracleEntry{
			MAC:     "aa:bb:cc:dd:ee:01",
			Hint:    ContextHint{ExpectedType: IfTypeEthernetCsmacd},
			IfIndex: 1,
		})
	}
	// 20 lookups with no hint → UnresolvedAmbiguous.
	for i := 0; i < 20; i++ {
		oracle = append(oracle, OracleEntry{MAC: "aa:bb:cc:dd:ee:01", IfIndex: 101})
	}

	rep := a.Run(oracle)
	assert.Equal(t, 100, rep.Sample)
	assert.Equal(t, 80, rep.MatchedCorrect)
	assert.Equal(t, 20, rep.UnresolvedAmbiguous)
	assert.InDelta(t, 0.0, rep.MisResolutionRate(), 1e-9, "PRD guardrail: <0.5%")
	assert.InDelta(t, 0.8, rep.CoverageRate(), 1e-9)
}
